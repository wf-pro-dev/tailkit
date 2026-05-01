package tailkit

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/tsnet"
)

// ServerConfig holds configuration for a tailkit-managed tsnet server.
type ServerConfig struct {
	Hostname  string
	AuthKey   string
	StateDir  string
	Ephemeral bool
}

// Server is a tailkit-managed tsnet server.
type Server struct {
	*tsnet.Server

	httpTransport    *http.Transport
	httpClient       *http.Client
	streamHTTPClient *http.Client

	closeOnce sync.Once
	closeErr  error
}

// NewServer constructs and starts a tsnet server.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Hostname == "" {
		return nil, fmt.Errorf("tailkit: ServerConfig.Hostname must not be empty")
	}

	authKey := cfg.AuthKey
	if authKey == "" {
		authKey = os.Getenv("TS_AUTHKEY")
	}

	stateDir := cfg.StateDir
	if stateDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			base = os.TempDir()
		}
		stateDir = base + "/tailkit/" + cfg.Hostname
	}

	ts := &tsnet.Server{
		Hostname:  cfg.Hostname,
		AuthKey:   authKey,
		Dir:       stateDir,
		Ephemeral: cfg.Ephemeral,
	}

	if err := ts.Start(); err != nil {
		return nil, fmt.Errorf("tailkit: failed to start tsnet server: %w", err)
	}

	srv := newServer(ts)

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		_ = srv.Close()
	}()

	return srv, nil
}

func newServer(ts *tsnet.Server) *Server {
	transport := &http.Transport{
		DialContext:         ts.Dial,
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Server{
		Server:        ts,
		httpTransport: transport,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		},
		// Long-lived SSE streams must be bounded by request context rather than
		// http.Client.Timeout, which applies to the full response body lifetime.
		streamHTTPClient: &http.Client{
			Transport: transport,
		},
	}
}

// HTTPClient returns the shared HTTP client used for outbound peer requests.
func (s *Server) HTTPClient() *http.Client {
	if s == nil {
		return nil
	}
	return s.httpClient
}

// StreamHTTPClient returns the shared HTTP client used for long-lived streams.
func (s *Server) StreamHTTPClient() *http.Client {
	if s == nil {
		return nil
	}
	return s.streamHTTPClient
}

// Close shuts down the shared HTTP transport and underlying tsnet server.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}

	s.closeOnce.Do(func() {
		if s.httpTransport != nil {
			s.httpTransport.CloseIdleConnections()
		}
		if s.Server != nil {
			s.closeErr = s.Server.Close()
		}
	})

	return s.closeErr
}

// localClient returns the *local.Client for this server.
// Returns nil if the local client cannot be obtained — callers must nil-check.
func (s *Server) localClient() *local.Client {
	lc, err := s.Server.LocalClient()
	if err != nil {
		return nil
	}
	return lc
}

// TLSConfig returns a *tls.Config using Tailscale-issued certificates.
func (s *Server) TLSConfig() *tls.Config {
	lc := s.localClient()
	if lc == nil {
		return &tls.Config{
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				return nil, fmt.Errorf("tailkit: local client unavailable")
			},
		}
	}
	return &tls.Config{
		GetCertificate: lc.GetCertificate,
	}
}

// ListenAndServeTLS starts an HTTPS server on the tsnet listener.
func (s *Server) ListenAndServeTLS(addr string, handler http.Handler) error {
	ln, err := s.Server.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("tailkit: listen %s: %w", addr, err)
	}
	httpSrv := &http.Server{Handler: handler, TLSConfig: s.TLSConfig()}
	return httpSrv.ServeTLS(ln, "", "")
}

// ListenAndServe starts a plain HTTP server on the tsnet listener.
func (s *Server) ListenAndServe(addr string, handler http.Handler) error {
	ln, err := s.Server.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("tailkit: listen %s: %w", addr, err)
	}
	return http.Serve(ln, handler)
}

// ─── Context key ─────────────────────────────────────────────────────────────

func withCallerIdentity(ctx context.Context, id CallerIdentity) context.Context {
	return context.WithValue(ctx, callerKey{}, id)
}
