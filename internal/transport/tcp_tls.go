package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
)

// TLSConfig holds certificate paths for TLS transports.
type TLSConfig struct {
	CertFile string // PEM certificate
	KeyFile  string // PEM private key
	CAFile   string // PEM CA bundle for peer verification; "" = system roots
}

// TCPTLS is a TCP transport with TLS.
type TCPTLS struct {
	cfg TLSConfig
}

// NewTCPTLS creates a new TCP+TLS transport.
func NewTCPTLS(cfg TLSConfig) *TCPTLS {
	return &TCPTLS{cfg: cfg}
}

// Dial establishes a TLS-over-TCP connection to addr.
func (t *TCPTLS) Dial(ctx context.Context, addr string) (net.Conn, error) {
	tlsCfg, err := t.buildClientTLS()
	if err != nil {
		return nil, err
	}
	d := &tls.Dialer{Config: tlsCfg}
	return d.DialContext(ctx, "tcp", addr)
}

// Listen creates a TLS listener on addr.
func (t *TCPTLS) Listen(addr string) (net.Listener, error) {
	tlsCfg, err := t.buildServerTLS()
	if err != nil {
		return nil, err
	}
	ln, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("tcp+tls: listen on %s: %w", addr, err)
	}
	return ln, nil
}

// Protocol returns "tcp+tls".
func (t *TCPTLS) Protocol() string {
	return "tcp+tls"
}

func (t *TCPTLS) buildClientTLS() (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if err := t.loadCerts(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (t *TCPTLS) buildServerTLS() (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequireAnyClientCert, // peer must present a cert
	}
	if err := t.loadCerts(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (t *TCPTLS) loadCerts(cfg *tls.Config) error {
	// Load our own cert+key
	if t.cfg.CertFile != "" && t.cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(t.cfg.CertFile, t.cfg.KeyFile)
		if err != nil {
			return fmt.Errorf("tcp+tls: loading cert/key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	// Load CA bundle for peer verification
	if t.cfg.CAFile != "" {
		pem, err := os.ReadFile(t.cfg.CAFile)
		if err != nil {
			return fmt.Errorf("tcp+tls: reading CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("tcp+tls: no valid certs in CA file %q", t.cfg.CAFile)
		}
		cfg.RootCAs = pool
		cfg.ClientCAs = pool
	}
	return nil
}
