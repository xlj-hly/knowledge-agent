package bootstrap

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeCancelsAndJoinsWorkerWhenHTTPFails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	joined := make(chan struct{})
	worker := func(ctx context.Context) error {
		<-ctx.Done()
		close(joined)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = serve(ctx, &http.Server{Addr: listener.Addr().String(), ReadHeaderTimeout: time.Second}, func(context.Context) error { return nil }, worker)
	if err == nil {
		t.Fatal("HTTP 端口被占用时应保留启动错误")
	}
	select {
	case <-joined:
	default:
		t.Fatal("serve 返回前没有停止并等待后台任务")
	}
}

func TestServeJoinsWorkerOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	joined := make(chan struct{})
	worker := func(ctx context.Context) error {
		cancel()
		<-ctx.Done()
		close(joined)
		return nil
	}
	err := serve(ctx, &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second}, func(context.Context) error { return nil }, worker)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("退出时后台任务仍在使用进程资源")
	}
}
