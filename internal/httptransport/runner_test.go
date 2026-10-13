package httptransport

import (
	"bufio"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
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
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/filetoolsserver"
)

func makeRunnerTestCertificate(t *testing.T, key crypto.Signer) ([]byte, []byte) {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
}

func TestTLSCertificateKeyStrength(t *testing.T) {
	weakRSA, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	strongRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var strong, weak tls.Certificate
	for _, tc := range []struct {
		name  string
		key   crypto.Signer
		valid bool
	}{
		{name: "weak RSA", key: weakRSA},
		{name: "RSA 2048", key: strongRSA, valid: true},
		{name: "ECDSA P-256", key: ecdsaKey, valid: true},
		{name: "Ed25519", key: edKey, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			certPEM, keyPEM := makeRunnerTestCertificate(t, tc.key)
			pair, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateTLSCertificateKeyStrength(pair); (err == nil) != tc.valid {
				t.Fatalf("validation error = %v, want valid = %t", err, tc.valid)
			}
			switch tc.name {
			case "weak RSA":
				weak = pair
			case "RSA 2048":
				strong = pair
			}
		})
	}
	for _, tc := range []struct {
		name        string
		certificate tls.Certificate
	}{
		{name: "empty chain"},
		{name: "invalid DER", certificate: tls.Certificate{Certificate: [][]byte{[]byte("not DER")}}},
		{name: "weak intermediate", certificate: tls.Certificate{
			Certificate: [][]byte{strong.Certificate[0], weak.Certificate[0]},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateTLSCertificateKeyStrength(tc.certificate); err == nil {
				t.Fatal("invalid TLS certificate chain was accepted")
			}
		})
	}
}

func TestRunnerRejectsWeakTLSCertificateAndClosesListener(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := makeRunnerTestCertificate(t, key)
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "server-cert.pem"), filepath.Join(dir, "server-key.pem")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := validTestConfig(2)
	cfg.TLSCertFile, cfg.TLSKeyFile = certFile, keyFile
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = (Runner{Config: cfg, Listener: listener}).Run(
		ctx, filetoolsserver.BuildServer(filetoolsserver.ServerOptions{Version: "runner"}),
	)
	if err == nil || !strings.Contains(err.Error(), "insufficient") {
		t.Fatalf("weak TLS certificate was not rejected at startup: %v", err)
	}
	if strings.Contains(err.Error(), certFile) || strings.Contains(err.Error(), keyFile) {
		t.Fatalf("TLS certificate error leaked configured paths: %v", err)
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("listener should be closed after rejected TLS certificate: %v", err)
	}
}

func TestRunnerRedactsTLSFilePaths(t *testing.T) {
	cfg := validTestConfig(2)
	cfg.TLSCertFile = "private-cert-path.pem"
	cfg.TLSKeyFile = "private-key-path.pem"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	err = (Runner{Config: cfg, Listener: listener}).Run(
		context.Background(),
		filetoolsserver.BuildServer(filetoolsserver.ServerOptions{Version: "runner"}),
	)
	if err == nil {
		t.Fatal("invalid TLS files were accepted")
	}
	if strings.Contains(err.Error(), cfg.TLSCertFile) || strings.Contains(err.Error(), cfg.TLSKeyFile) {
		t.Fatalf("TLS error leaked configured paths: %v", err)
	}
}

func TestRunnerServesHTTPAndEnforcesHeaderLimit(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	cfg := validTestConfig(4)
	cfg.AllowedHosts = map[string]struct{}{strings.ToLower(address): {}}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- (Runner{Config: cfg, Listener: listener}).Run(
			ctx,
			filetoolsserver.BuildServer(filetoolsserver.ServerOptions{Version: "runner-e2e"}),
		)
	}()

	baseURL := "http://" + address
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, requestErr := http.Get(baseURL + "/readyz")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("runner did not become ready: %v", requestErr)
		}
		time.Sleep(20 * time.Millisecond)
	}

	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(connection, "GET /healthz HTTP/1.1\r\nHost: %s\r\nX-Large: %s\r\n\r\n", address, strings.Repeat("x", 80*1024))
	if err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	statusLine, err := bufio.NewReader(connection).ReadString('\n')
	_ = connection.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusLine, "431") {
		t.Fatalf("oversized-header status = %q, want 431", statusLine)
	}

	clientCtx, clientCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer clientCancel()
	session := connectHTTPClient(t, clientCtx, baseURL+cfg.Path)
	tools, err := session.ListTools(clientCtx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if got, want := len(tools.Tools), expectedToolCount(); got != want {
		t.Fatalf("tool count = %d, want %d", got, want)
	}
	_ = session.Close()
	clientCancel()

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("runner did not stop")
	}
}

func TestRunnerStopsOnContextCancellation(t *testing.T) {
	cfg := validTestConfig(2)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runner := Runner{Config: cfg, Listener: listener}
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, filetoolsserver.BuildServer(filetoolsserver.ServerOptions{Version: "runner"}))
	}()

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not stop")
	}
}
