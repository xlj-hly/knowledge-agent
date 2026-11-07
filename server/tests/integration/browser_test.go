package saas_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// TestSaaSBrowserServer 为显式启用的 Playwright 套件提供真实 API。
// 先独占测试监听器再创建可回收 schema；调用方必须按本次运行标识握手和停止。
func TestSaaSBrowserServer(t *testing.T) {
	addr := os.Getenv("E2E_SERVER_ADDR")
	if addr == "" {
		t.Skip("browser test harness not enabled")
	}
	if addr != "127.0.0.1:8081" && addr != "127.0.0.1:8082" {
		t.Fatal("browser harness only binds 127.0.0.1:8081 or 127.0.0.1:8082")
	}
	runID := os.Getenv("E2E_RUN_ID")
	if runID == "" {
		t.Fatal("browser tests require E2E_RUN_ID")
	}
	if os.Getenv("SAAS_TEST_DATABASE_URL") == "" {
		t.Fatal("browser tests require an explicit disposable database")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
	})
	s := newIntegrationSuite(t)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerErr := make(chan error, 1)
	go func() { workerErr <- s.application.Knowledge.Run(workerCtx) }()
	t.Cleanup(func() {
		stopWorker()
		select {
		case err := <-workerErr:
			if err != nil {
				t.Errorf("knowledge worker: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("knowledge worker did not stop before database cleanup")
		}
	})
	done := make(chan struct{})
	var stopOnce sync.Once
	mux := http.NewServeMux()
	mux.Handle("/", s.handler)
	registerBrowserModelFixture(mux)
	mux.HandleFunc("GET /__test/identity", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"run_id": runID}); err != nil {
			t.Errorf("write test identity: %v", err)
		}
	})
	mux.HandleFunc("POST /__test/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-E2E-Run-ID") != runID {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		stopOnce.Do(func() { close(done) })
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close browser server: %v", err)
		}
	})
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	serveExited := false
	select {
	case <-done:
	case err := <-errCh:
		serveExited = true
		if err != http.ErrServerClosed {
			t.Error(err)
		}
	case <-time.After(12 * time.Minute):
		t.Error("browser test harness timed out")
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		t.Error(err)
		if err := server.Close(); err != nil {
			t.Errorf("force-close browser server: %v", err)
		}
	}
	if !serveExited {
		select {
		case err := <-errCh:
			if err != http.ErrServerClosed {
				t.Error(err)
			}
		case <-shutdown.Done():
			t.Error("browser harness did not stop")
		}
	}
}
