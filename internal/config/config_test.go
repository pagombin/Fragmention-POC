package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const minimalYAML = `
mongo:
  uri: "mongodb://localhost:27017/?replicaSet=rs0"
auth:
  enabled: true
  bearer_token: "abcdefghijklmnopqrstuvwxyz0123456789"
`

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestLoad_MinimalValid(t *testing.T) {
	p := writeCfg(t, minimalYAML)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "mongodb://localhost:27017/?replicaSet=rs0", cfg.Mongo.ResolvedURI)
	require.False(t, cfg.Mongo.IsSRV)
	require.Equal(t, 10*time.Second, cfg.Mongo.ConnectTimeout)
}

func TestLoad_SRVBumpsConnectTimeout(t *testing.T) {
	p := writeCfg(t, `
mongo:
  uri: "mongodb+srv://u:p@cluster.example.com/?authSource=admin"
auth:
  enabled: true
  bearer_token: "abcdefghijklmnopqrstuvwxyz0123456789"
`)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.True(t, cfg.Mongo.IsSRV)
	require.Equal(t, 30*time.Second, cfg.Mongo.ConnectTimeout)
}

func TestLoad_EnvOverride(t *testing.T) {
	p := writeCfg(t, minimalYAML)
	t.Setenv("MFPOC_LOGGING_LEVEL", "debug")
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "debug", cfg.Logging.Level)
}

func TestLoad_SecretFromEnv(t *testing.T) {
	p := writeCfg(t, `
mongo:
  uri: "mongodb://localhost:27017/"
auth:
  enabled: true
  bearer_token: "${env:TOK}"
`)
	t.Setenv("TOK", "abcdefghijklmnopqrstuvwxyz0123456789")

	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "abcdefghijklmnopqrstuvwxyz0123456789", cfg.Auth.BearerToken)
}

func TestLoad_SecretFromFile(t *testing.T) {
	dir := t.TempDir()
	tokFile := filepath.Join(dir, "tok")
	require.NoError(t, os.WriteFile(tokFile, []byte("abcdefghijklmnopqrstuvwxyz0123456789\n"), 0o600))
	p := writeCfg(t, `
mongo:
  uri: "mongodb://localhost:27017/"
auth:
  enabled: true
  bearer_token: "${file:`+tokFile+`}"
`)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "abcdefghijklmnopqrstuvwxyz0123456789", cfg.Auth.BearerToken)
}

func TestLoad_RejectsShortToken(t *testing.T) {
	p := writeCfg(t, `
mongo:
  uri: "mongodb://localhost:27017/"
auth:
  enabled: true
  bearer_token: "tooshort"
`)
	_, err := Load(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least 32 bytes")
}

func TestLoad_MissingMongo(t *testing.T) {
	p := writeCfg(t, `
auth:
  enabled: true
  bearer_token: "abcdefghijklmnopqrstuvwxyz0123456789"
`)
	_, err := Load(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mongo.uri")
}

func TestLoad_InvalidScheme(t *testing.T) {
	p := writeCfg(t, `
mongo:
  uri: "http://localhost:27017/"
auth:
  enabled: true
  bearer_token: "abcdefghijklmnopqrstuvwxyz0123456789"
`)
	_, err := Load(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported scheme")
}

func TestRedactMongoURI(t *testing.T) {
	cases := map[string]string{
		"mongodb://u:p@localhost:27017/?password=abc": "mongodb://u:****@localhost:27017/?password=****",
		"mongodb+srv://u:p@cluster.example.com/":      "mongodb+srv://u:****@cluster.example.com/",
		"mongodb://localhost:27017/":                  "mongodb://localhost:27017/",
	}
	for in, want := range cases {
		got := RedactMongoURI(in)
		require.Equal(t, want, got, "input=%s", in)
		require.False(t, strings.Contains(got, ":p@"), "unredacted password slipped through: %s", got)
	}
}

func TestBuildURIFromParts(t *testing.T) {
	p := writeCfg(t, `
mongo:
  host: localhost
  port: 27017
  username: alice
  password: hunter2
  replica_set: rs0
  auth_source: admin
auth:
  enabled: true
  bearer_token: "abcdefghijklmnopqrstuvwxyz0123456789"
`)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Contains(t, cfg.Mongo.ResolvedURI, "mongodb://alice:hunter2@localhost:27017/")
	require.Contains(t, cfg.Mongo.ResolvedURI, "replicaSet=rs0")
	require.Contains(t, cfg.Mongo.ResolvedURI, "authSource=admin")
}
