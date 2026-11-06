package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDecodeRejectsNullAndNonObjects(t *testing.T) {
	for name, body := range map[string]string{
		"null":          `null`,
		"array":         `[]`,
		"string":        `"text"`,
		"null nickname": `{"nickname":null}`,
		"null avatar":   `{"nickname":"ok","avatar_url":null}`,
		"unknown field": `{"unknown":"value"}`,
		"two objects":   `{"nickname":"ok"}{"nickname":"changed"}`,
		"oversized":     `{"nickname":"` + strings.Repeat("a", 16*1024) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPut, "/account/profile", strings.NewReader(body))
			var in struct {
				Nickname  string `json:"nickname"`
				AvatarURL string `json:"avatar_url"`
			}
			if err := decode(c, &in); err == nil {
				t.Fatalf("accepted invalid JSON shape: %s", body)
			}
		})
	}
}

func TestIdentityUsesRequestHost(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "http://alpha.test/api/tenant", nil)
	c.Request.Header.Set("X-Tenant-Hostname", "beta.test")
	id, err := identity(c)
	if err != nil || id.Hostname != "alpha.test" {
		t.Fatalf("client header overrode tenant entry: %#v %v", id, err)
	}
}

func TestPlatformRoutesRejectTenantHostBeforeAuthentication(t *testing.T) {
	r := New(injectedDependencies(), Config{})
	req := httptest.NewRequest(http.MethodPost, "http://alpha.test/api/platform/auth/login", strings.NewReader(`null`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("platform route accepted non-platform entry: %d %s", w.Code, w.Body.String())
	}
}

func TestProfileRouteRejectsPatch(t *testing.T) {
	r := New(injectedDependencies(), Config{})
	req := httptest.NewRequest(http.MethodPatch, "/api/account/profile", strings.NewReader(`{"nickname":"昵称","avatar_url":""}`))
	req.Header.Set("X-Tenant-Hostname", "alpha.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unexpected profile method accepted: HTTP %d", w.Code)
	}
}

func TestRateLimitRetainsCORSAndSecurityHeaders(t *testing.T) {
	r := New(injectedDependencies(), Config{AllowedOrigins: []string{"https://app.alpha.test"}})
	for i := 0; i < 31; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/platform/auth/login", strings.NewReader(`null`))
		req.RemoteAddr = "192.0.2.1:12345"
		req.Header.Set("Origin", "https://app.alpha.test")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if i == 30 {
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("status=%d", w.Code)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "https://app.alpha.test" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("rate limit lost headers: %v", w.Header())
			}
		}
	}
	preflight := httptest.NewRequest(http.MethodOptions, "/api/platform/auth/login", nil)
	preflight.RemoteAddr = "192.0.2.1:12345"
	preflight.Header.Set("Origin", "https://app.alpha.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, preflight)
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight counted as auth attempt: %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.alpha.test" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("preflight lost headers: %v", w.Header())
	}
	denied := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
	denied.Header.Set("Origin", "https://untrusted.test")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, denied)
	if w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("denied preflight contract: %d %v", w.Code, w.Header())
	}
}
