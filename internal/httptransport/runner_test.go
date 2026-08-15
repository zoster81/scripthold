package httptransport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/filetoolsserver"
)

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
