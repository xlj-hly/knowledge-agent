package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

type requestMarker struct{}

type injectedAuth struct {
	AuthUseCases
	login func(context.Context, dto.TenantIdentity, dto.LoginInput) (dto.TenantSession, error)
	me    func(context.Context, dto.TenantIdentity) (dto.TenantSession, error)
}

func (s injectedAuth) Login(ctx context.Context, id dto.TenantIdentity, in dto.LoginInput) (dto.TenantSession, error) {
	return s.login(ctx, id, in)
}

func (s injectedAuth) Me(ctx context.Context, id dto.TenantIdentity) (dto.TenantSession, error) {
	return s.me(ctx, id)
}

type injectedAccounts struct {
	AccountUseCases
	profile func(context.Context, dto.TenantIdentity, dto.ProfileInput) (dto.User, error)
}

func (s injectedAccounts) Profile(ctx context.Context, id dto.TenantIdentity, in dto.ProfileInput) (dto.User, error) {
	return s.profile(ctx, id, in)
}

type injectedSites struct{ SiteUseCases }

func (injectedSites) IsPlatformHost(host string) bool { return host == "platform.test" }

type injectedCompanies struct{ CompanyUseCases }
type injectedMaintenance struct{ MaintenanceUseCases }

func injectedDependencies() Dependencies {
	return Dependencies{
		Auth: injectedAuth{}, Accounts: injectedAccounts{}, Companies: injectedCompanies{},
		Sites: injectedSites{}, Maintenance: injectedMaintenance{},
	}
}

func TestInjectedUseCasesReceiveRequestIdentityAndContext(t *testing.T) {
	deps := injectedDependencies()
	called := false
	deps.Auth = injectedAuth{login: func(ctx context.Context, id dto.TenantIdentity, in dto.LoginInput) (dto.TenantSession, error) {
		called = true
		if ctx.Value(requestMarker{}) != "login" || id.Hostname != "alpha.test" || id.Token != "raw-token" {
			t.Fatalf("request identity or context lost: id=%+v marker=%v", id, ctx.Value(requestMarker{}))
		}
		if in.Username != "member" || in.Password != "password-value" {
			t.Fatalf("request input lost: %+v", in)
		}
		return dto.TenantSession{User: dto.User{ID: "42", TenantID: "7"}, Tenant: dto.Tenant{ID: "7"}}, nil
	}}
	r := New(deps, Config{})
	req := httptest.NewRequest(http.MethodPost, "http://alpha.test/api/auth/login", strings.NewReader(`{"username":"member","password":"password-value"}`))
	req = req.WithContext(context.WithValue(req.Context(), requestMarker{}, "login"))
	req.Header.Set("Authorization", "Bearer raw-token")
	req.Header.Set("X-Tenant-Hostname", "beta.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !called || w.Code != http.StatusOK {
		t.Fatalf("injected login not executed: called=%v status=%d body=%s", called, w.Code, w.Body.String())
	}
	var body struct {
		Data dto.TenantSession `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.User.ID != "42" || body.Data.Tenant.ID != "7" {
		t.Fatalf("use case response not mapped: %s; %v", w.Body.String(), err)
	}
}

func TestInjectedAccountFailureKeepsHTTPContract(t *testing.T) {
	deps := injectedDependencies()
	called := false
	deps.Accounts = injectedAccounts{profile: func(ctx context.Context, id dto.TenantIdentity, in dto.ProfileInput) (dto.User, error) {
		called = true
		if ctx.Value(requestMarker{}) != "profile" || id.Hostname != "alpha.test" || id.Token != "profile-token" || in.Nickname == nil || *in.Nickname != "昵称" || in.AvatarURL == nil || *in.AvatarURL != "" {
			t.Fatalf("account input or identity lost: id=%+v input=%+v", id, in)
		}
		return dto.User{}, domain.Forbidden()
	}}
	r := New(deps, Config{})
	req := httptest.NewRequest(http.MethodPut, "http://alpha.test/api/account/profile", strings.NewReader(`{"nickname":"昵称","avatar_url":""}`))
	req = req.WithContext(context.WithValue(req.Context(), requestMarker{}, "profile"))
	req.Header.Set("Authorization", "Bearer profile-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !called || w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"code":"forbidden"`) {
		t.Fatalf("injected failure contract changed: called=%v status=%d body=%s", called, w.Code, w.Body.String())
	}
}

type unreadBody struct{ read bool }

func (b *unreadBody) Read([]byte) (int, error) {
	b.read = true
	return 0, errors.New("request body must not be read before authentication")
}

func (*unreadBody) Close() error { return nil }

func TestAvatarAuthenticationPrecedesRequestBodyRead(t *testing.T) {
	deps := injectedDependencies()
	deps.Auth = injectedAuth{me: func(_ context.Context, id dto.TenantIdentity) (dto.TenantSession, error) {
		if id.Hostname != "alpha.test" || id.Token != "invalid-token" {
			t.Fatalf("avatar authentication identity lost: %+v", id)
		}
		return dto.TenantSession{}, domain.Unauthorized()
	}}
	r := New(deps, Config{})
	req := httptest.NewRequest(http.MethodPost, "http://alpha.test/api/account/avatar", nil)
	body := &unreadBody{}
	req.Body = body
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if body.read || w.Code != http.StatusUnauthorized {
		t.Fatalf("avatar body read before rejection: read=%v status=%d body=%s", body.read, w.Code, w.Body.String())
	}
}
