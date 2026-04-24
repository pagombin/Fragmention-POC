package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/pagombin/fragmention-poc/internal/config"
)

// Server is the application's HTTP(S) server with graceful lifecycle.
type Server struct {
	cfg    *config.Config
	logger zerolog.Logger
	http   *http.Server
	tls    bool
	addr   string
}

// NewServer wraps an http.Handler in the configured server, applying TLS and
// listener validation. It does not start listening; call Start.
func NewServer(cfg *config.Config, logger zerolog.Logger, handler http.Handler) (*Server, error) {
	addr := cfg.Server.Listen
	if addr == "" {
		addr = "0.0.0.0:8080"
	}

	if err := checkInsecurePublicBind(cfg); err != nil {
		return nil, err
	}

	hs := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: durationOrDefault(cfg.Server.ReadHeaderTimeout, 10*time.Second),
		ReadTimeout:       durationOrDefault(cfg.Server.ReadTimeout, 30*time.Second),
		WriteTimeout:      durationOrDefault(cfg.Server.WriteTimeout, 60*time.Second),
		IdleTimeout:       durationOrDefault(cfg.Server.IdleTimeout, 120*time.Second),
	}

	srv := &Server{cfg: cfg, logger: logger, http: hs, tls: cfg.Server.TLS.Enabled, addr: addr}
	return srv, nil
}

func durationOrDefault(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

// checkInsecurePublicBind rejects public-IP HTTP binds unless the operator
// explicitly set insecure_allow_http_on_public. Loopback and unix sockets are
// allowed without TLS. This implements § 10 of the spec.
func checkInsecurePublicBind(cfg *config.Config) error {
	if cfg.Server.TLS.Enabled {
		return nil
	}
	if cfg.Server.InsecureAllowHTTPOnPublic {
		return nil
	}
	host, _, err := net.SplitHostPort(cfg.Server.Listen)
	if err != nil {
		return nil // tolerated; server.ListenAndServe will surface it
	}
	if host == "" || host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return nil
	}
	// Treat "0.0.0.0" and any non-loopback IP as public-facing.
	return errors.New(
		"server bound to a public address without TLS; enable server.tls or set server.insecure_allow_http_on_public=true",
	)
}

// Start begins serving. It blocks until ctx is cancelled, then shuts the
// server down gracefully within the configured shutdown timeout.
func (s *Server) Start(ctx context.Context) error {
	scheme := "http"
	listenErr := make(chan error, 1)

	if s.tls {
		tlsCfg, err := s.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("tls config: %w", err)
		}
		s.http.TLSConfig = tlsCfg
		scheme = "https"
		go func() {
			// cert/key paths are embedded in tlsCfg; pass empty strings.
			if err := s.http.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				listenErr <- err
			}
			close(listenErr)
		}()
	} else {
		go func() {
			if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				listenErr <- err
			}
			close(listenErr)
		}()
	}
	s.logger.Info().Str("addr", s.addr).Str("scheme", scheme).Msg("http_listener_started")

	select {
	case <-ctx.Done():
	case err, ok := <-listenErr:
		if ok && err != nil {
			return err
		}
	}

	shutdownTimeout := durationOrDefault(s.cfg.Server.ShutdownTimeout, 30*time.Second)
	shCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.http.Shutdown(shCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	s.logger.Info().Msg("http_listener_stopped")
	return nil
}

func (s *Server) buildTLSConfig() (*tls.Config, error) {
	t := s.cfg.Server.TLS
	if t.CertFile != "" && t.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load keypair: %w", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
	}
	if t.SelfSigned {
		certFile := filepath.Join(t.DataDir, "selfsigned.crt")
		keyFile := filepath.Join(t.DataDir, "selfsigned.key")
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			s.logger.Warn().Str("data_dir", t.DataDir).Msg("generating self-signed TLS certificate")
			if err := generateSelfSigned(t.DataDir, certFile, keyFile); err != nil {
				return nil, err
			}
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load self-signed: %w", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
	}
	return nil, errors.New("tls enabled but no cert source configured")
}

// generateSelfSigned writes a new ECDSA self-signed certificate + key pair to
// the given paths. The certificate is valid for 5 years and has the droplet's
// local addresses as SANs so browsers can at least identify the host.
func generateSelfSigned(dir, certFile, keyFile string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir tls dir: %w", err)
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("serial: %w", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "mfpoc-self-signed"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(5 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ipnet.IP)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return nil
}
