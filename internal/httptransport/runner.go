package httptransport

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	readHeaderTimeout  = 5 * time.Second
	requestReadTimeout = 5 * time.Minute
	httpIdleTimeout    = 2 * time.Minute
	shutdownTimeout    = 15 * time.Second
	maxHeaderBytes     = 64 * 1024
)

// Runner owns the native HTTP listener and graceful lifecycle.
type Runner struct {
	Config   Config
	Logger   *slog.Logger
	Listener net.Listener // optional test or embedding listener
}

func (runner Runner) Run(ctx context.Context, server *mcp.Server) error {
	listener := runner.Listener
	var err error
	if listener == nil {
		listener, err = net.Listen("tcp", runner.Config.Address)
		if err != nil {
			return err
		}
	}

	if runner.Config.UseTLS() {
		certificate, err := tls.LoadX509KeyPair(runner.Config.TLSCertFile, runner.Config.TLSKeyFile)
		if err != nil {
			_ = listener.Close()
			return errors.New("failed to load HTTP TLS certificate or key")
		}
		if err := validateTLSCertificateKeyStrength(certificate); err != nil {
			_ = listener.Close()
			return err
		}
		listener = tls.NewListener(listener, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
		})
	}

	handler := NewHandler(runner.Config, server, runner.Logger)
	httpServer := &http.Server{
		Addr:              runner.Config.Address,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       requestReadTimeout,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          log.New(io.Discard, "", 0),
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	handler.setReady(true)
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()

	select {
	case err := <-serveErr:
		handler.close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		handler.beginShutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		shutdownErr := httpServer.Shutdown(shutdownCtx)
		cancel()
		if shutdownErr != nil {
			_ = httpServer.Close()
		}
		handler.close()
		serveResult := <-serveErr
		if shutdownErr != nil {
			return shutdownErr
		}
		if serveResult != nil && !errors.Is(serveResult, http.ErrServerClosed) {
			return serveResult
		}
		return ctx.Err()
	}
}

// Validate every certificate the server presents before accepting TLS connections.
func validateTLSCertificateKeyStrength(certificate tls.Certificate) error {
	if len(certificate.Certificate) == 0 {
		return errors.New("HTTP TLS certificate chain is empty")
	}
	for _, der := range certificate.Certificate {
		parsed, err := x509.ParseCertificate(der)
		if err != nil {
			return errors.New("HTTP TLS certificate chain contains an invalid certificate")
		}
		strong := false
		switch key := parsed.PublicKey.(type) {
		case *rsa.PublicKey:
			strong = key.N != nil && key.N.BitLen() >= 2048
		case *ecdsa.PublicKey:
			strong = key.Curve != nil && key.Curve.Params() != nil && key.Curve.Params().BitSize >= 224
		case ed25519.PublicKey:
			strong = len(key) == ed25519.PublicKeySize
		}
		if !strong {
			return errors.New("HTTP TLS certificate chain contains an insufficient or unsupported public key")
		}
	}
	return nil
}
