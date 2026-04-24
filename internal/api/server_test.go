package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pagombin/fragmention-poc/internal/config"
)

func baseCfg() *config.Config {
	c := config.Example()
	c.Auth.Enabled = false
	c.Server.Listen = "127.0.0.1:0"
	return c
}

func TestCheckInsecurePublicBind_AllowsLoopback(t *testing.T) {
	c := baseCfg()
	require.NoError(t, checkInsecurePublicBind(c))
}

func TestCheckInsecurePublicBind_RejectsPublicHTTP(t *testing.T) {
	c := baseCfg()
	c.Server.Listen = "0.0.0.0:8080"
	err := checkInsecurePublicBind(c)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insecure_allow_http_on_public")
}

func TestCheckInsecurePublicBind_RespectsOverride(t *testing.T) {
	c := baseCfg()
	c.Server.Listen = "0.0.0.0:8080"
	c.Server.InsecureAllowHTTPOnPublic = true
	require.NoError(t, checkInsecurePublicBind(c))
}

func TestCheckInsecurePublicBind_TLSNotChecked(t *testing.T) {
	c := baseCfg()
	c.Server.Listen = "0.0.0.0:8443"
	c.Server.TLS.Enabled = true
	require.NoError(t, checkInsecurePublicBind(c))
}

func TestGenerateSelfSigned(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "c.pem")
	key := filepath.Join(dir, "k.pem")
	require.NoError(t, generateSelfSigned(dir, cert, key))

	info, err := os.Stat(cert)
	require.NoError(t, err)
	require.NotZero(t, info.Size())
	info, err = os.Stat(key)
	require.NoError(t, err)
	require.NotZero(t, info.Size())
	// Key file must be mode 0600 for minimal hardening.
	require.Equal(t, os.FileMode(0o600), info.Mode()&0o777)
}
