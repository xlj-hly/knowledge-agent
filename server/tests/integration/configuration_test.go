package saas_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"go-server/internal/app/saas/port"
)

type wechatConfigView struct {
	AppID     string `json:"app_id"`
	HasSecret bool   `json:"has_secret"`
}

func TestSaaSDisabledTenantKeepsSiteIdentityAndRejectsLogin(t *testing.T) {
	s := newIntegrationSuite(t)
	alpha := s.createTenant(t, "alpha.test")
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID, s.platform.Token, map[string]any{"status": "disabled"}))
	site := decodeSuccess[struct {
		Kind   string      `json:"kind"`
		Tenant *tenantView `json:"tenant"`
	}](t, s.request(http.MethodGet, "alpha.test", "/api/site", "", nil))
	if site.Kind != "tenant" || site.Tenant == nil || site.Tenant.ID != alpha.Tenant.ID || site.Tenant.Status != "disabled" {
		t.Fatalf("disabled tenant lost its bound site identity: %+v", site)
	}
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/login", "", map[string]any{"username": "tenant.admin", "password": adminPassword}), http.StatusForbidden)
}

func TestSaaSSiteAndTenantWeChatConfiguration(t *testing.T) {
	s := newIntegrationSuite(t)
	alpha := s.createTenant(t, "alpha.test")
	beta := s.createTenant(t, "beta.test")
	for host, kind := range map[string]string{"platform.localhost": "platform", "ALPHA.TEST:8080": "tenant", "unknown.test": "unknown"} {
		site := decodeSuccess[struct {
			Kind, Hostname string
			Tenant         *tenantView `json:"tenant"`
		}](t, s.request(http.MethodGet, host, "/api/site", "", nil))
		if site.Kind != kind || site.Hostname != strings.ToLower(strings.Split(host, ":")[0]) {
			t.Fatalf("incorrect site %s: %+v", host, site)
		}
		if (kind == "tenant") != (site.Tenant != nil) {
			t.Fatal("site returned wrong tenant data")
		}
	}
	endpoint := "/api/platform/tenants/" + alpha.Tenant.ID + "/wechat"
	initial := decodeSuccess[wechatConfigView](t, s.request(http.MethodGet, "platform.test", endpoint, s.platform.Token, nil))
	if initial.AppID != "" || initial.HasSecret {
		t.Fatal("new tenant unexpectedly configured")
	}
	for _, body := range []map[string]any{{"app_id": "wx0000000000000001"}, {"app_id": "wx0000000000000001", "app_secret": ""}, {"app_id": "wx0000000000000001", "app_secret": nil}, {"app_id": "invalid", "app_secret": "secret"}, {"app_id": "wx0000000000000001", "app_secret": "   "}, {"app_id": "wx0000000000000001", "app_secret": "valid", "enabled": true}} {
		assertFailure(t, s.request(http.MethodPut, "platform.test", endpoint, s.platform.Token, body), 400)
	}
	secret := "configuration_secret_alpha"
	saved := decodeSuccess[wechatConfigView](t, s.request(http.MethodPut, "platform.test", "/api/platform/tenants/00"+alpha.Tenant.ID+"/wechat", s.platform.Token, map[string]any{"app_id": "wx0000000000000001", "app_secret": secret}))
	if saved.AppID != "wx0000000000000001" || !saved.HasSecret {
		t.Fatal("configuration was not saved")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stored string
	var version int64
	if err := s.pool.QueryRow(ctx, "SELECT app_secret,wechat_config_version FROM company WHERE id=$1", alpha.Tenant.ID).Scan(&stored, &version); err != nil {
		t.Fatal(err)
	}
	if stored != secret || version != 1 {
		t.Fatal("company did not store the supplied secret with initial version")
	}
	for _, body := range []map[string]any{{"app_id": saved.AppID}, {"app_id": saved.AppID, "app_secret": ""}} {
		decodeSuccess[wechatConfigView](t, s.request(http.MethodPut, "platform.test", endpoint, s.platform.Token, body))
	}
	assertFailure(t, s.request(http.MethodPut, "platform.test", endpoint, s.platform.Token, map[string]any{"app_id": "wx0000000000000002"}), 400)
	read := s.request(http.MethodGet, "platform.test", endpoint, s.platform.Token, nil)
	if strings.Contains(string(read.body), secret) || strings.Contains(string(read.body), "app_secret") {
		t.Fatal("read response exposed stored credentials")
	}
	other := decodeSuccess[wechatConfigView](t, s.request(http.MethodGet, "platform.test", "/api/platform/tenants/"+beta.Tenant.ID+"/wechat", s.platform.Token, nil))
	if other.HasSecret {
		t.Fatal("tenant configurations crossed scopes")
	}
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	assertFailure(t, s.request(http.MethodPut, "platform.test", endpoint, admin.Token, map[string]any{"app_id": saved.AppID, "app_secret": "forged"}), 401)
	assertFailure(t, s.request(http.MethodGet, "alpha.test", endpoint, s.platform.Token, nil), 403)
	assertFailure(t, s.request(http.MethodPost, "platform.test", "/api/platform/tenants", s.platform.Token, map[string]any{"name": "reserved", "hostname": "platform.localhost", "admin_username": "new.admin", "admin_password": adminPassword}), 400)
}

func TestSaaSWeChatConfigurationRotationRejectsInflightIdentity(t *testing.T) {
	s := newIntegrationSuite(t)
	alpha := s.createTenant(t, "alpha.test")
	started := make(chan struct{})
	resume := make(chan struct{})
	configureWeChat(t, s, func(_ context.Context, c port.WeChatCredentials, code string) (port.WeChatIdentity, error) {
		if code == "blocked_snapshot" {
			close(started)
			<-resume
		}
		return port.WeChatIdentity{AppID: c.AppID, OpenID: "identity_" + code}, nil
	})
	result := make(chan apiResult, 1)
	go func() {
		result <- s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": "blocked_snapshot"})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("login did not reach upstream")
	}
	decodeSuccess[wechatConfigView](t, s.request(http.MethodPut, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID+"/wechat", s.platform.Token, map[string]any{"app_id": "wx0000000000000001", "app_secret": "rotated_secret"}))
	close(resume)
	select {
	case value := <-result:
		assertWeChatError(t, value, 409, "conflict")
	case <-time.After(5 * time.Second):
		t.Fatal("login did not finish")
	}
	if s.count(t, "SELECT count(*) FROM users WHERE open_id<>''") != 0 || s.count(t, "SELECT count(*) FROM users") != 1 {
		t.Fatal("stale configuration created identity or user")
	}
	first := wechatLogin(t, s, "alpha.test", "fresh")
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID, s.platform.Token, map[string]any{"hostname": "alpha-new.test"}))
	next := wechatLogin(t, s, "alpha-new.test", "fresh")
	if next.User.ID != first.User.ID {
		t.Fatal("changing domain lost tenant-owned WeChat configuration")
	}
	decodeSuccess[wechatConfigView](t, s.request(http.MethodPut, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID+"/wechat", s.platform.Token, map[string]any{"app_id": "wx0000000000000002", "app_secret": "new_application_secret"}))
	newApplication := wechatLogin(t, s, "alpha-new.test", "fresh")
	if newApplication.User.ID == first.User.ID || s.count(t, "SELECT count(*) FROM users WHERE open_id<>''") != 2 {
		t.Fatal("changing AppID merged unrelated OpenID identities or discarded the old account")
	}
}

func TestSaaSWeChatPasswordRequiresSameVerifiedIdentity(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "alpha.test")
	configureWeChat(t, s, fixtureIdentity)
	first := wechatLogin(t, s, "alpha.test", "person_a")
	second := wechatLogin(t, s, "alpha.test", "person_a")
	other := wechatLogin(t, s, "alpha.test", "person_b")
	change := func(token, code, password string) apiResult {
		return s.request(http.MethodPost, "alpha.test", "/api/account/wechat/password", token, map[string]any{"code": code, "new_password": password})
	}
	assertFailure(t, change(first.Token, "person_b", memberPassword), 403)
	assertFailure(t, change("", "person_a", memberPassword), 401)
	assertFailure(t, change(first.Token, "person_a", "short"), 400)
	decodeSuccess[map[string]bool](t, change(first.Token, "person_a", memberPassword))
	for _, token := range []string{first.Token, second.Token} {
		assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", token, nil), 401)
	}
	decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", other.Token, nil))
	passwordLogin := s.login(t, "alpha.test", first.User.Username, memberPassword)
	if passwordLogin.User.ID != first.User.ID {
		t.Fatal("new password did not authenticate original account")
	}
	wechatLogin(t, s, "alpha.test", "person_a")
	if after := s.login(t, "alpha.test", first.User.Username, memberPassword); after.User.ID != first.User.ID {
		t.Fatal("repeat WeChat login reset user's password")
	}
}
