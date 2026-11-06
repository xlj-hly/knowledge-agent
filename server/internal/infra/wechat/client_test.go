package wechat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const successBody = `{"openid":"openid_for_verified_user","session_key":"MDEyMzQ1Njc4OWFiY2RlZg==","unionid":"optional_unionid"}`

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := New()
	client.endpoint = server.URL
	return client
}

func credentials() Credentials {
	return Credentials{AppID: "wx0123456789abcdef", AppSecret: "test-secret-not-for-output"}
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("expected %s, received %T", code, err)
	}
	for _, secret := range []string{credentials().AppSecret, "temporary-login-code", "upstream diagnostic"} {
		if strings.Contains(fmt.Sprintf("%+v", err), secret) {
			t.Fatal("provider error exposed sensitive external data")
		}
	}
}

func TestExchangeSendsOfficialParametersAndReturnsOnlyIdentity(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || q.Get("appid") != credentials().AppID || q.Get("secret") != credentials().AppSecret || q.Get("js_code") != "temporary-login-code" || q.Get("grant_type") != "authorization_code" || len(q) != 4 {
			t.Error("unexpected code2session request contract")
		}
		_, _ = w.Write([]byte(successBody))
	})
	identity, err := client.Exchange(context.Background(), credentials(), "temporary-login-code")
	if err != nil {
		t.Fatalf("valid response rejected: %T", err)
	}
	if identity.AppID != credentials().AppID || identity.OpenID != "openid_for_verified_user" {
		t.Fatalf("wrong verified identity: %#v", identity)
	}
}

func TestExchangeClassifiesFailuresWithoutReturningUpstreamDetails(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		code   string
	}{
		"invalid code":         {200, `{"errcode":40029,"errmsg":"upstream diagnostic"}`, "invalid_code"},
		"consumed code":        {200, `{"errcode":40163,"errmsg":"upstream diagnostic"}`, "invalid_code"},
		"blocked account":      {200, `{"errcode":40226,"errmsg":"upstream diagnostic"}`, "blocked"},
		"WeChat rate limit":    {200, `{"errcode":45011,"errmsg":"upstream diagnostic"}`, "rate_limited"},
		"HTTP rate limit":      {429, "upstream diagnostic", "rate_limited"},
		"system busy":          {200, `{"errcode":-1,"errmsg":"upstream diagnostic"}`, "unavailable"},
		"upstream unavailable": {503, "upstream diagnostic", "unavailable"},
		"bad app credentials":  {200, `{"errcode":40013,"errmsg":"upstream diagnostic"}`, "upstream_error"},
		"HTTP rejection":       {400, "upstream diagnostic", "upstream_error"},
		"non JSON":             {200, "upstream diagnostic", "upstream_error"},
		"non object":           {200, `[]`, "upstream_error"},
		"null":                 {200, `null`, "upstream_error"},
		"trailing object":      {200, successBody + `{}`, "upstream_error"},
		"missing identity":     {200, `{"session_key":"MDEyMzQ1Njc4OWFiY2RlZg=="}`, "upstream_error"},
		"wrong identity type":  {200, `{"openid":42,"session_key":"MDEyMzQ1Njc4OWFiY2RlZg=="}`, "upstream_error"},
		"null identity":        {200, `{"openid":null,"session_key":"MDEyMzQ1Njc4OWFiY2RlZg=="}`, "upstream_error"},
		"whitespace identity":  {200, `{"openid":"other user","session_key":"MDEyMzQ1Njc4OWFiY2RlZg=="}`, "upstream_error"},
		"missing session key":  {200, `{"openid":"some_user"}`, "upstream_error"},
		"invalid session key":  {200, `{"openid":"some_user","session_key":"notbase64"}`, "upstream_error"},
		"wrong key length":     {200, `{"openid":"some_user","session_key":"c2hvcnQ="}`, "upstream_error"},
		"null errcode":         {200, `{"errcode":null,"openid":"some_user","session_key":"MDEyMzQ1Njc4OWFiY2RlZg=="}`, "upstream_error"},
		"wrong errcode type":   {200, `{"errcode":"40029"}`, "upstream_error"},
		"oversized response":   {200, strings.Repeat("x", 16*1024+1), "upstream_error"},
	} {
		t.Run(name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			identity, err := client.Exchange(context.Background(), credentials(), "temporary-login-code")
			requireCode(t, err, tc.code)
			if identity != (Identity{}) {
				t.Fatal("failure returned a usable identity")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestExchangeRedactsTransportErrorsAndHonorsCancellation(t *testing.T) {
	client := New()
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("failed request %s upstream diagnostic", r.URL.String())
	})
	_, err := client.Exchange(context.Background(), credentials(), "temporary-login-code")
	requireCode(t, err, "unavailable")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = New().Exchange(ctx, credentials(), "temporary-login-code")
	requireCode(t, err, "unavailable")
	if !errors.Is(err, context.Canceled) {
		t.Fatal("caller cancellation cause was lost")
	}
}

func TestExchangeHasOwnDeadlineAndRejectsRedirects(t *testing.T) {
	client := New()
	client.timeout = 20 * time.Millisecond
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	_, err := client.Exchange(context.Background(), credentials(), "temporary-login-code")
	requireCode(t, err, "unavailable")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("provider deadline cause was lost")
	}
	redirected := false
	client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/unexpected" {
			redirected = true
			_, _ = w.Write([]byte(successBody))
			return
		}
		http.Redirect(w, r, "/unexpected", http.StatusFound)
	})
	_, err = client.Exchange(context.Background(), credentials(), "temporary-login-code")
	requireCode(t, err, "upstream_error")
	if redirected {
		t.Fatal("credential-bearing request followed a redirect")
	}
}

func TestExchangeRejectsInvalidInputBeforeSendingRequest(t *testing.T) {
	requested := false
	client := testClient(t, func(http.ResponseWriter, *http.Request) { requested = true })
	_, err := client.Exchange(context.Background(), Credentials{}, "temporary-login-code")
	requireCode(t, err, "unavailable")
	for _, code := range []string{"", " code", "line\nbreak", strings.Repeat("x", 513)} {
		_, err := client.Exchange(context.Background(), credentials(), code)
		requireCode(t, err, "invalid_code")
	}
	if requested {
		t.Fatal("invalid local input reached the upstream API")
	}
}
