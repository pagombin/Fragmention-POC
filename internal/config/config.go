// Package config loads, validates, and exposes the application's runtime
// configuration. It supports YAML files with environment variable overrides
// (prefix MFPOC_) and understands secret references of the form
// ${env:NAME} and ${file:/path}. MongoDB URIs may be either standard
// (mongodb://) or DNS seedlist (mongodb+srv://); both forms are honored and
// passed to the driver unmodified.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the fully-validated top-level configuration.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Mongo     MongoConfig     `mapstructure:"mongo"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Loader    LoaderConfig    `mapstructure:"loader"`
	Deleter   DeleterConfig   `mapstructure:"deleter"`
	Collector CollectorConfig `mapstructure:"collector"`
	Compact   CompactConfig   `mapstructure:"compact"`
	Workload  WorkloadConfig  `mapstructure:"workload"`
	Auth      AuthConfig      `mapstructure:"auth"`
	POC       POCConfig       `mapstructure:"poc"`
}

// ServerConfig controls the HTTP/HTTPS listener and TLS.
type ServerConfig struct {
	Listen                    string        `mapstructure:"listen"`
	TLS                       TLSConfig     `mapstructure:"tls"`
	HTTPRedirectListen        string        `mapstructure:"http_redirect_listen"`
	InsecureAllowHTTPOnPublic bool          `mapstructure:"insecure_allow_http_on_public"`
	MaxRequestBytes           int64         `mapstructure:"max_request_bytes"`
	ReadHeaderTimeout         time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout               time.Duration `mapstructure:"read_timeout"`
	WriteTimeout              time.Duration `mapstructure:"write_timeout"`
	IdleTimeout               time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout           time.Duration `mapstructure:"shutdown_timeout"`
	RateLimitPerMinute        int           `mapstructure:"rate_limit_per_minute"`
	RateLimitFailedAuthPerMin int           `mapstructure:"rate_limit_failed_auth_per_minute"`
}

// TLSConfig controls transport security.
type TLSConfig struct {
	Enabled    bool         `mapstructure:"enabled"`
	CertFile   string       `mapstructure:"cert_file"`
	KeyFile    string       `mapstructure:"key_file"`
	SelfSigned bool         `mapstructure:"self_signed"`
	ACME       ACMEConfig   `mapstructure:"acme"`
	DataDir    string       `mapstructure:"data_dir"`
}

// ACMEConfig configures Let's Encrypt certificate issuance.
type ACMEConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Domain   string `mapstructure:"domain"`
	Email    string `mapstructure:"email"`
	CacheDir string `mapstructure:"cache_dir"`
}

// MongoConfig holds target-cluster connection details.
type MongoConfig struct {
	URI              string        `mapstructure:"uri"`
	Host             string        `mapstructure:"host"`
	Port             int           `mapstructure:"port"`
	Username         string        `mapstructure:"username"`
	Password         string        `mapstructure:"password"`
	ReplicaSet       string        `mapstructure:"replica_set"`
	AuthSource       string        `mapstructure:"auth_source"`
	TLS              bool          `mapstructure:"tls"`
	CAFile           string        `mapstructure:"ca_file"`
	ClientCertFile   string        `mapstructure:"client_cert_file"`
	ConnectTimeout   time.Duration `mapstructure:"connect_timeout"`
	OperationTimeout time.Duration `mapstructure:"operation_timeout"`
	MaxPoolSize      uint64        `mapstructure:"max_pool_size"`
	MinPoolSize      uint64        `mapstructure:"min_pool_size"`
	AppName          string        `mapstructure:"app_name"`
	// ResolvedURI is the final URI the driver will use, produced at Load time.
	ResolvedURI string `mapstructure:"-"`
	// IsSRV is true when the resolved URI is an mongodb+srv:// seedlist.
	IsSRV bool `mapstructure:"-"`
}

// StorageConfig controls the SQLite state store.
type StorageConfig struct {
	Path                  string        `mapstructure:"path"`
	BusyTimeout           time.Duration `mapstructure:"busy_timeout"`
	MetricsRetentionDays  int           `mapstructure:"metrics_retention_days"`
	ClusterMetricsRetDays int           `mapstructure:"cluster_metrics_retention_days"`
	JanitorInterval       time.Duration `mapstructure:"janitor_interval"`
}

// LoggingConfig controls logger level and format.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// LoaderConfig sets loader defaults. Per-operation invocations may override.
type LoaderConfig struct {
	DefaultWorkers         int     `mapstructure:"default_workers"`
	DefaultBatchSize       int     `mapstructure:"default_batch_size"`
	DefaultWriteConcern    string  `mapstructure:"default_write_concern"`
	StorageHeadroomPercent float64 `mapstructure:"storage_headroom_percent"`
}

// DeleterConfig sets deletion defaults.
type DeleterConfig struct {
	DefaultBatchSize int           `mapstructure:"default_batch_size"`
	PreviewTTL       time.Duration `mapstructure:"preview_ttl"`
	MaxRatio         float64       `mapstructure:"max_ratio"`
	InterBatchJitter time.Duration `mapstructure:"inter_batch_jitter"`
}

// CollectorConfig sets metrics collector defaults.
type CollectorConfig struct {
	IdleInterval   time.Duration `mapstructure:"idle_interval"`
	ActiveInterval time.Duration `mapstructure:"active_interval"`
	BackoffInitial time.Duration `mapstructure:"backoff_initial"`
	BackoffMax     time.Duration `mapstructure:"backoff_max"`
}

// CompactConfig sets compact-orchestrator defaults.
type CompactConfig struct {
	MaxReplicationLag    time.Duration `mapstructure:"max_replication_lag"`
	StepdownWaitTimeout  time.Duration `mapstructure:"stepdown_wait_timeout"`
	ValidateAfterCompact bool          `mapstructure:"validate_after_compact"`
}

// WorkloadConfig sets defaults for the workload generator.
type WorkloadConfig struct {
	DefaultTargetOpsPerSec int     `mapstructure:"default_target_ops_per_sec"`
	DefaultReadWeight      float64 `mapstructure:"default_read_weight"`
	DefaultWriteWeight     float64 `mapstructure:"default_write_weight"`
	DefaultAggregateWeight float64 `mapstructure:"default_aggregate_weight"`
}

// AuthConfig controls API authentication.
type AuthConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	BearerToken string `mapstructure:"bearer_token"`
	BasicUser   string `mapstructure:"basic_user"`
	BasicPass   string `mapstructure:"basic_pass"`
}

// POCConfig holds POC-wide invariants used by safety gates.
type POCConfig struct {
	DatabasePrefix string `mapstructure:"database_prefix"`
}

// Load reads configuration from the supplied path (optional) and environment
// and returns a fully validated Config. An empty path means "no config file;
// rely on defaults + environment" - useful for container deployments.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("MFPOC")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	applyDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	var cfg Config
	// viper registers StringToTimeDurationHookFunc by default, so time.Duration
	// fields accept "10s" / "1m30s" strings without a custom hook.
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	if err := resolveSecrets(&cfg); err != nil {
		return nil, fmt.Errorf("resolve secrets: %w", err)
	}
	if err := cfg.finalize(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(v *viper.Viper) {
	v.SetDefault("server.listen", "0.0.0.0:8080")
	v.SetDefault("server.tls.enabled", false)
	v.SetDefault("server.tls.self_signed", false)
	v.SetDefault("server.tls.data_dir", "./data/tls")
	v.SetDefault("server.tls.acme.cache_dir", "./data/acme")
	v.SetDefault("server.max_request_bytes", 1<<20)
	v.SetDefault("server.read_header_timeout", "10s")
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "60s")
	v.SetDefault("server.idle_timeout", "120s")
	v.SetDefault("server.shutdown_timeout", "30s")
	v.SetDefault("server.rate_limit_per_minute", 60)
	v.SetDefault("server.rate_limit_failed_auth_per_minute", 10)
	v.SetDefault("server.insecure_allow_http_on_public", false)

	v.SetDefault("storage.path", "./data/mfpoc.db")
	v.SetDefault("storage.busy_timeout", "5s")
	v.SetDefault("storage.metrics_retention_days", 30)
	v.SetDefault("storage.cluster_metrics_retention_days", 365)
	v.SetDefault("storage.janitor_interval", "24h")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	v.SetDefault("mongo.connect_timeout", "10s")
	v.SetDefault("mongo.operation_timeout", "30s")
	v.SetDefault("mongo.max_pool_size", 100)
	v.SetDefault("mongo.min_pool_size", 0)
	v.SetDefault("mongo.app_name", "mfpoc")
	v.SetDefault("mongo.auth_source", "admin")

	v.SetDefault("loader.default_workers", 0) // 0 => 2 * NumCPU at runtime
	v.SetDefault("loader.default_batch_size", 1000)
	v.SetDefault("loader.default_write_concern", "1")
	v.SetDefault("loader.storage_headroom_percent", 30.0)

	v.SetDefault("deleter.default_batch_size", 1000)
	v.SetDefault("deleter.preview_ttl", "2m")
	v.SetDefault("deleter.max_ratio", 0.95)
	v.SetDefault("deleter.inter_batch_jitter", "25ms")

	v.SetDefault("collector.idle_interval", "10s")
	v.SetDefault("collector.active_interval", "2s")
	v.SetDefault("collector.backoff_initial", "1s")
	v.SetDefault("collector.backoff_max", "30s")

	v.SetDefault("compact.max_replication_lag", "10s")
	v.SetDefault("compact.stepdown_wait_timeout", "60s")
	v.SetDefault("compact.validate_after_compact", false)

	v.SetDefault("workload.default_target_ops_per_sec", 100)
	v.SetDefault("workload.default_read_weight", 0.7)
	v.SetDefault("workload.default_write_weight", 0.2)
	v.SetDefault("workload.default_aggregate_weight", 0.1)

	v.SetDefault("auth.enabled", true)
	v.SetDefault("poc.database_prefix", "poc_db_")
}

var secretRef = regexp.MustCompile(`^\$\{(env|file):([^}]+)\}$`)

// resolveSecrets expands ${env:VAR} and ${file:/path} references in-place.
// Anything else is passed through unchanged.
func resolveSecrets(cfg *Config) error {
	resolve := func(target *string) error {
		if target == nil {
			return nil
		}
		val := strings.TrimSpace(*target)
		m := secretRef.FindStringSubmatch(val)
		if m == nil {
			return nil
		}
		kind, ref := m[1], m[2]
		switch kind {
		case "env":
			v, ok := os.LookupEnv(ref)
			if !ok {
				return fmt.Errorf("env secret %q not set", ref)
			}
			*target = v
		case "file":
			data, err := os.ReadFile(filepath.Clean(ref))
			if err != nil {
				return fmt.Errorf("file secret %q: %w", ref, err)
			}
			*target = strings.TrimRight(string(data), "\r\n")
		default:
			return fmt.Errorf("unknown secret kind %q", kind)
		}
		return nil
	}
	for _, field := range []*string{
		&cfg.Mongo.URI,
		&cfg.Mongo.Password,
		&cfg.Auth.BearerToken,
		&cfg.Auth.BasicPass,
	} {
		if err := resolve(field); err != nil {
			return err
		}
	}
	return nil
}

// finalize composes the effective Mongo URI (preferring the explicit field)
// and records whether the target is an SRV seedlist.
func (c *Config) finalize() error {
	uri := strings.TrimSpace(c.Mongo.URI)
	if uri == "" && c.Mongo.Host != "" {
		built, err := buildURI(c.Mongo)
		if err != nil {
			return fmt.Errorf("build mongo URI: %w", err)
		}
		uri = built
	}
	if uri == "" {
		return nil // defer to Validate()
	}
	c.Mongo.ResolvedURI = uri
	c.Mongo.IsSRV = strings.HasPrefix(strings.ToLower(uri), "mongodb+srv://")
	// For SRV, the spec requires a longer default connect timeout if the user
	// did not override the default.
	if c.Mongo.IsSRV && c.Mongo.ConnectTimeout <= 10*time.Second {
		c.Mongo.ConnectTimeout = 30 * time.Second
	}
	return nil
}

func buildURI(m MongoConfig) (string, error) {
	host := m.Host
	if m.Port > 0 {
		host = fmt.Sprintf("%s:%d", m.Host, m.Port)
	}
	u := url.URL{Scheme: "mongodb", Host: host, Path: "/"}
	if m.Username != "" || m.Password != "" {
		u.User = url.UserPassword(m.Username, m.Password)
	}
	q := url.Values{}
	if m.ReplicaSet != "" {
		q.Set("replicaSet", m.ReplicaSet)
	}
	if m.AuthSource != "" {
		q.Set("authSource", m.AuthSource)
	}
	if m.TLS {
		q.Set("tls", "true")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Validate enforces startup invariants. It reports a single aggregated error
// so operators see every problem at once.
func (c *Config) Validate() error {
	var errs []string
	errs = append(errs, c.validateMongo()...)
	errs = append(errs, c.validateCore()...)
	errs = append(errs, c.validateTLS()...)
	errs = append(errs, c.validateAuth()...)
	if len(errs) > 0 {
		return errors.New("config invalid: " + strings.Join(errs, "; "))
	}
	return nil
}

func (c *Config) validateMongo() []string {
	var errs []string
	if c.Mongo.URI == "" && c.Mongo.Host == "" {
		errs = append(errs, "mongo.uri or mongo.host must be set")
	}
	if c.Mongo.ResolvedURI != "" {
		if _, err := url.Parse(c.Mongo.ResolvedURI); err != nil {
			errs = append(errs, fmt.Sprintf("mongo.uri is not a valid URL: %v", err))
		}
		scheme := strings.ToLower(strings.SplitN(c.Mongo.ResolvedURI, "://", 2)[0])
		if scheme != "mongodb" && scheme != "mongodb+srv" {
			errs = append(errs, fmt.Sprintf("mongo.uri has unsupported scheme %q", scheme))
		}
	}
	return errs
}

func (c *Config) validateCore() []string {
	var errs []string
	if c.Storage.Path == "" {
		errs = append(errs, "storage.path must be set")
	}
	if !validLevel(c.Logging.Level) {
		errs = append(errs, fmt.Sprintf("logging.level %q invalid (debug|info|warn|error)", c.Logging.Level))
	}
	if c.Deleter.MaxRatio <= 0 || c.Deleter.MaxRatio > 0.95 {
		errs = append(errs, "deleter.max_ratio must be in (0, 0.95]")
	}
	if c.Loader.StorageHeadroomPercent < 0 || c.Loader.StorageHeadroomPercent > 90 {
		errs = append(errs, "loader.storage_headroom_percent must be in [0, 90]")
	}
	return errs
}

func (c *Config) validateTLS() []string {
	if !c.Server.TLS.Enabled {
		return nil
	}
	var errs []string
	hasExplicitCert := c.Server.TLS.CertFile != "" && c.Server.TLS.KeyFile != ""
	if !hasExplicitCert && !c.Server.TLS.SelfSigned && !c.Server.TLS.ACME.Enabled {
		errs = append(errs, "server.tls.enabled requires one of: cert_file+key_file, self_signed, or acme.enabled")
	}
	if c.Server.TLS.ACME.Enabled && c.Server.TLS.ACME.Domain == "" {
		errs = append(errs, "server.tls.acme.enabled requires acme.domain")
	}
	return errs
}

func (c *Config) validateAuth() []string {
	if !c.Auth.Enabled {
		return nil
	}
	var errs []string
	token := c.Auth.BearerToken
	if token == "" && c.Auth.BasicUser == "" {
		errs = append(errs, "auth.enabled requires bearer_token or basic_user")
	}
	if token != "" && len([]byte(token)) < 32 {
		errs = append(errs, "auth.bearer_token must be at least 32 bytes")
	}
	return errs
}

func validLevel(s string) bool {
	switch strings.ToLower(s) {
	case "", "trace", "debug", "info", "warn", "warning", "error":
		return true
	}
	return false
}

// Example returns a *Config populated with safe defaults, for tests and for
// printing a sample to new operators. It does not perform secret resolution.
func Example() *Config {
	v := viper.New()
	applyDefaults(v)
	var c Config
	_ = v.Unmarshal(&c)
	c.Mongo.URI = "mongodb://localhost:27017/?replicaSet=rs0"
	c.Auth.BearerToken = "replace-with-32-plus-bytes-of-random-hex-xx"
	_ = c.finalize()
	return &c
}
