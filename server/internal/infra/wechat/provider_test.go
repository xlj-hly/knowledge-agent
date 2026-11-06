package wechat_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"go-server/internal/app/saas/port"
	"go-server/internal/infra/wechat"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func withWechatTransport(t *testing.T, transport transportFunc) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
}

func TestWeChatAdapterMapsVerifiedIdentity(t *testing.T) {
	withWechatTransport(t, func(request *http.Request) (*http.Response, error) {
		query := request.URL.Query()
		if query.Get("appid") != "wx1234567890abcdef" || query.Get("secret") != "test-secret" || query.Get("js_code") != "fresh_code" {
			t.Fatal("business credentials or code were not passed to the protocol client")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"openid":"verified_person","session_key":"MDEyMzQ1Njc4OWFiY2RlZg=="}`)), Header: make(http.Header)}, nil
	})
	identity, err := wechat.NewProvider(wechat.New()).Exchange(context.Background(), port.WeChatCredentials{AppID: "wx1234567890abcdef", AppSecret: "test-secret"}, "fresh_code")
	if err != nil || identity.AppID != "wx1234567890abcdef" || identity.OpenID != "verified_person" {
		t.Fatalf("verified identity was not mapped: %+v %v", identity, err)
	}
}

func TestWeChatAdapterMapsProtocolFailures(t *testing.T) {
	for upstreamCode, expected := range map[int]string{40029: "invalid_code", 40226: "blocked", 45011: "rate_limited", -1: "unavailable", 123456: "upstream_error"} {
		t.Run(expected, func(t *testing.T) {
			withWechatTransport(t, func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"errcode":%d}`, upstreamCode))), Header: make(http.Header)}, nil
			})
			_, err := wechat.NewProvider(wechat.New()).Exchange(context.Background(), port.WeChatCredentials{AppID: "test-app", AppSecret: "secret"}, "code")
			var failure *port.WeChatError
			if !errors.As(err, &failure) || failure.Code != expected {
				t.Fatalf("wrong failure classification: %T %v", err, err)
			}
		})
	}
}

func TestWeChatAdapterPreservesCancellationWithoutSecrets(t *testing.T) {
	withWechatTransport(t, func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := wechat.NewProvider(wechat.New()).Exchange(ctx, port.WeChatCredentials{AppID: "test-app", AppSecret: "sensitive-secret"}, "sensitive-code")
	var failure *port.WeChatError
	if !errors.As(err, &failure) || failure.Code != "unavailable" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not preserved: %T %v", err, err)
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if strings.Contains(current.Error(), "sensitive") || strings.Contains(current.Error(), "https://") {
			t.Fatal("mapped failure exposed request details")
		}
	}
}
