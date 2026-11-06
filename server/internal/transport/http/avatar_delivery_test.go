package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/storage"
)

type avatarReadCases struct {
	AccountUseCases
	object storage.Object
}

func (s avatarReadCases) OpenAvatar(context.Context, string, string, string) (storage.Object, error) {
	return s.object, nil
}

type trackedReadBody struct {
	io.Reader
	closed bool
}

func (b *trackedReadBody) Close() error { b.closed = true; return nil }

func TestAvatarHeadAndRangeKeepObjectResponseContract(t *testing.T) {
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		t.Run(method, func(t *testing.T) {
			body := &trackedReadBody{Reader: strings.NewReader("0123456789")}
			deps := injectedDependencies()
			deps.Accounts = avatarReadCases{object: storage.Object{Body: body, Metadata: storage.Metadata{Size: 10, ContentType: "image/png"}}}
			req := httptest.NewRequest(method, "http://alpha.test/api/avatars/1/2/"+strings.Repeat("a", 32)+".png", nil)
			if method == http.MethodGet {
				req.Header.Set("Range", "bytes=2-4")
			}
			w := httptest.NewRecorder()
			New(deps, Config{}).ServeHTTP(w, req)
			if !body.closed {
				t.Fatal("对象响应体未关闭")
			}
			if method == http.MethodHead {
				if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "10" {
					t.Fatalf("HEAD契约改变：status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
				}
			} else if w.Code != 206 || w.Body.String() != "234" || w.Header().Get("Content-Range") != "bytes 2-4/10" {
				t.Fatalf("Range契约改变：status=%d body=%q", w.Code, w.Body.String())
			}
		})
	}
}

func TestAvatarHeadPreflightAllowsConfiguredOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "http://alpha.test/api/avatars/1/2/"+strings.Repeat("a", 32)+".png", nil)
	req.Header.Set("Origin", "https://web.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodHead)
	req.Header.Set("Access-Control-Request-Headers", "Authorization")
	w := httptest.NewRecorder()
	New(injectedDependencies(), Config{AllowedOrigins: []string{"https://web.example.com"}}).ServeHTTP(w, req)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "https://web.example.com" || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), http.MethodHead) {
		t.Fatalf("HEAD预检未获允许：status=%d headers=%v", w.Code, w.Header())
	}
}
