// Package mongo wraps the MongoDB v2 driver with the app-specific concerns:
// connecting with the resolved URI (standard or SRV), verifying reachability,
// enforcing the supported-version gate, and exposing topology/stats helpers.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	cfgpkg "github.com/pagombin/fragmention-poc/internal/config"
)

// MinMajor is the lowest MongoDB major version mfpoc supports. Spec § 21.1
// mandates a hard refusal against pre-5.0 servers because compact semantics
// differ materially in older releases.
const MinMajor = 5

// Client wraps *mongo.Client together with the server-side facts we gather
// at connect time: build version, feature-compatibility version, whether
// the URI was an SRV seedlist, and so on. These facts are recorded against
// every run so cross-version comparisons are never ambiguous.
type Client struct {
	raw      *mongo.Client
	info     ServerInfo
	isSRV    bool
	redacted string
}

// ServerInfo summarises the facts learned about the target cluster during
// Connect(). Pointer fields like StorageEngine may be empty on older servers.
type ServerInfo struct {
	Version           string
	Major, Minor, Patch int
	FCV               string
	StorageEngine     string
	WiredTigerCodec   string // collection compressor (block_compressor)
	GitVersion        string
}

// Connect opens a client to the target cluster, pings it, and gathers the
// facts we need before any higher-level service runs. It rejects unsupported
// server versions. The returned Client is safe for concurrent use.
func Connect(ctx context.Context, c cfgpkg.MongoConfig) (*Client, error) {
	uri := strings.TrimSpace(c.ResolvedURI)
	if uri == "" {
		return nil, errors.New("mongo: resolved URI is empty")
	}
	connectTimeout := c.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 10 * time.Second
	}
	opts := options.Client().ApplyURI(uri).
		SetAppName(nonEmpty(c.AppName, "mfpoc")).
		SetConnectTimeout(connectTimeout).
		SetServerSelectionTimeout(connectTimeout).
		SetMaxPoolSize(defaultPool(c.MaxPoolSize, 100)).
		SetMinPoolSize(c.MinPoolSize)

	if c.OperationTimeout > 0 {
		opts = opts.SetTimeout(c.OperationTimeout)
	}

	cctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	raw, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("mongo.Connect: %w", err)
	}

	if err := raw.Ping(cctx, readpref.Primary()); err != nil {
		_ = raw.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo.Ping: %w", err)
	}

	info, err := gatherServerInfo(cctx, raw)
	if err != nil {
		_ = raw.Disconnect(context.Background())
		return nil, err
	}
	if info.Major < MinMajor {
		_ = raw.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo: server version %s is below minimum %d.0", info.Version, MinMajor)
	}

	return &Client{
		raw:      raw,
		info:     info,
		isSRV:    c.IsSRV,
		redacted: cfgpkg.RedactMongoURI(uri),
	}, nil
}

// Close disconnects the underlying driver. Idempotent and safe on nil.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Disconnect(ctx)
}

// Raw returns the underlying driver client. Reserved for advanced callers
// (integration tests); production code should use the typed helpers.
func (c *Client) Raw() *mongo.Client { return c.raw }

// ServerInfo returns the facts gathered at connect time.
func (c *Client) ServerInfo() ServerInfo { return c.info }

// RedactedURI returns the connection URI with any credentials masked.
func (c *Client) RedactedURI() string { return c.redacted }

// IsSRV reports whether the target was specified via mongodb+srv://.
func (c *Client) IsSRV() bool { return c.isSRV }

// Ping verifies connectivity against the primary. Used by /ready.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.raw == nil {
		return errors.New("mongo: client not open")
	}
	return c.raw.Ping(ctx, readpref.Primary())
}

func gatherServerInfo(ctx context.Context, raw *mongo.Client) (ServerInfo, error) {
	admin := raw.Database("admin")
	var bi bson.M
	if err := admin.RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&bi); err != nil {
		return ServerInfo{}, fmt.Errorf("buildInfo: %w", err)
	}
	info := ServerInfo{
		Version:    asString(bi["version"]),
		GitVersion: asString(bi["gitVersion"]),
	}
	info.Major, info.Minor, info.Patch = parseVersion(info.Version)
	if se, ok := bi["storageEngines"].(bson.A); ok && len(se) > 0 {
		info.StorageEngine = asString(se[0])
	}

	// featureCompatibilityVersion requires admin privilege; tolerate failure
	// on managed clusters that restrict it.
	var fcv bson.M
	if err := admin.RunCommand(ctx, bson.D{
		{Key: "getParameter", Value: 1},
		{Key: "featureCompatibilityVersion", Value: 1},
	}).Decode(&fcv); err == nil {
		if m, ok := fcv["featureCompatibilityVersion"].(bson.M); ok {
			info.FCV = asString(m["version"])
		}
	}

	// WiredTiger default codec lives in serverStatus, not buildInfo.
	var ss bson.M
	if err := admin.RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&ss); err == nil {
		if wt, ok := ss["wiredTiger"].(bson.M); ok {
			if c, ok := wt["concurrentTransactions"].(bson.M); ok {
				_ = c // silence unused; we surface concurrentTransactions via the collector later
			}
		}
	}
	return info, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func parseVersion(v string) (maj, minr, patch int) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) > 0 {
		maj = atoiSafe(parts[0])
	}
	if len(parts) > 1 {
		minr = atoiSafe(parts[1])
	}
	if len(parts) > 2 {
		patch = atoiSafe(strings.SplitN(parts[2], "-", 2)[0])
	}
	return
}

func atoiSafe(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func nonEmpty(v, dflt string) string {
	if v == "" {
		return dflt
	}
	return v
}

func defaultPool(v, dflt uint64) uint64 {
	if v == 0 {
		return dflt
	}
	return v
}
