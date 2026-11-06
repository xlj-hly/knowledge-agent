package saas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go-server/internal/app/saas/port"
)

type wechatFixture func(context.Context, port.WeChatCredentials, string) (port.WeChatIdentity, error)

func (f wechatFixture) Exchange(ctx context.Context, credentials port.WeChatCredentials, code string) (port.WeChatIdentity, error) {
	return f(ctx, credentials, code)
}

func configureWeChat(t *testing.T, s *integrationSuite, provider wechatFixture) {
	t.Helper()
	s.wechatProvider = provider
	s.wechatConfigured = true
	s.rebuildHandler(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, "SELECT id::text,hostname FROM company")
	if err != nil {
		t.Fatal(err)
	}
	type entry struct{ id, host string }
	var entries []entry
	for rows.Next() {
		var value entry
		if err := rows.Scan(&value.id, &value.host); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		entries = append(entries, value)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, value := range entries {
		saveFixtureWeChat(t, s, value.id, value.host)
	}
}

func saveFixtureWeChat(t *testing.T, s *integrationSuite, tenantID, host string) {
	t.Helper()
	appID := "wx0000000000000001"
	if host == "beta.test" {
		appID = "wx0000000000000002"
	}
	decodeSuccess[map[string]any](t, s.request(http.MethodPut, "platform.test", "/api/platform/tenants/"+tenantID+"/wechat", s.platform.Token, map[string]any{"app_id": appID, "app_secret": "test_secret_" + strings.ReplaceAll(host, ".", "_")}))
}

func fixtureIdentity(_ context.Context, credentials port.WeChatCredentials, code string) (port.WeChatIdentity, error) {
	return port.WeChatIdentity{AppID: credentials.AppID, OpenID: "openid_" + code}, nil
}

func wechatLogin(t *testing.T, s *integrationSuite, host, code string) loginView {
	t.Helper()
	result := s.request(http.MethodPost, host, "/api/auth/wechat/login", "", map[string]any{"code": code})
	for _, secret := range []string{"openid", "open_id", "wechat_app_id", "session_key", "app_secret", "test_secret"} {
		if bytes.Contains(result.body, []byte(secret)) {
			t.Fatalf("WeChat response exposes %s", secret)
		}
	}
	login := decodeSuccess[loginView](t, result)
	assertLogin(t, login)
	return login
}

func assertWeChatError(t *testing.T, result apiResult, status int, code string) {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.body, &body); err != nil || result.status != status || body.Error.Code != code {
		t.Fatalf("expected HTTP %d %s; got HTTP %d %s", status, code, result.status, result.body)
	}
}

func TestSaaSWeChatLoginCreatesAndReusesIsolatedUsers(t *testing.T) {
	s := newIntegrationSuite(t)
	configureWeChat(t, s, fixtureIdentity)
	alpha := s.createTenant(t, "alpha.test")
	s.createTenant(t, "beta.test")
	first := wechatLogin(t, s, "ALPHA.TEST:8080", "person_a")
	if first.User.Role != "user" || first.User.Nickname != "微信用户" || first.User.TenantID != alpha.Tenant.ID || first.User.Username == "openid_person_a" {
		t.Fatal("first WeChat login did not create an ordinary scoped account")
	}
	if s.count(t, "SELECT count(*) FROM users WHERE id=$1 AND password_hash LIKE '$2%'", first.User.ID) != 1 {
		t.Fatal("WeChat account did not receive a password hash")
	}
	if s.count(t, "SELECT count(*) FROM users WHERE tenant_id=$1 AND id=$2 AND wechat_app_id=$3 AND open_id=$4", alpha.Tenant.ID, first.User.ID, "wx0000000000000001", "openid_person_a") != 1 {
		t.Fatal("verified identity fields were not persisted on the user")
	}
	decodeSuccess[userView](t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", first.Token, map[string]any{"nickname": "微信资料保存", "avatar_url": ""}))
	second := wechatLogin(t, s, "alpha.test", "person_a")
	if second.User.ID != first.User.ID || second.User.Nickname != "微信资料保存" {
		t.Fatal("repeat WeChat login lost account or profile")
	}
	beta := wechatLogin(t, s, "beta.test", "person_a")
	if beta.User.ID == first.User.ID || beta.Tenant.ID == first.Tenant.ID {
		t.Fatal("WeChat identity crossed tenant boundaries")
	}
	assertFailure(t, s.request(http.MethodGet, "beta.test", "/api/auth/me", first.Token, nil), 401)
	assertFailure(t, s.request(http.MethodGet, "platform.test", "/api/platform/auth/me", first.Token, nil), 401)
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/login", "", map[string]any{"username": first.User.Username, "password": memberPassword}), 401)
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/account/password", first.Token, map[string]any{"current_password": memberPassword, "new_password": "Different-pass-2026"}), 400)
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "alpha.test", "/api/admin/users/"+first.User.ID+"/reset-password", admin.Token, map[string]any{"new_password": memberPassword}))
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", first.Token, nil), 401)
	if login := s.login(t, "alpha.test", first.User.Username, memberPassword); login.User.ID != first.User.ID {
		t.Fatal("administrator-set password did not enable the same account")
	}
	third := wechatLogin(t, s, "alpha.test", "person_a")
	decodeSuccess[userView](t, s.request(http.MethodPatch, "alpha.test", "/api/admin/users/"+first.User.ID, admin.Token, map[string]any{"status": "disabled"}))
	assertWeChatError(t, s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": "person_a"}), 403, "disabled")
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", third.Token, nil), 401)
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "beta.test", "/api/auth/logout", beta.Token, nil))
	assertFailure(t, s.request(http.MethodGet, "beta.test", "/api/auth/me", beta.Token, nil), 401)
}

func TestSaaSWeChatRegistrationAndFailureBoundaries(t *testing.T) {
	s := newIntegrationSuite(t)
	alpha := s.createTenant(t, "alpha.test")
	assertWeChatError(t, s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": "valid"}), 503, "wechat_not_configured")
	configureWeChat(t, s, func(ctx context.Context, c port.WeChatCredentials, code string) (port.WeChatIdentity, error) {
		if code == "invalid_code" || code == "blocked" || code == "rate_limited" || code == "unavailable" || code == "upstream_error" {
			return port.WeChatIdentity{}, port.NewWeChatError(code, nil)
		}
		return fixtureIdentity(ctx, c, code)
	})
	for code, expected := range map[string]struct {
		status int
		code   string
	}{"invalid_code": {401, "wechat_invalid_code"}, "blocked": {403, "wechat_login_blocked"}, "rate_limited": {429, "wechat_rate_limited"}, "unavailable": {503, "wechat_unavailable"}, "upstream_error": {502, "wechat_upstream_error"}} {
		assertWeChatError(t, s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": code}), expected.status, expected.code)
	}
	if s.count(t, "SELECT count(*) FROM users") != 1 || s.count(t, "SELECT count(*) FROM users WHERE open_id<>''") != 0 || s.count(t, "SELECT count(*) FROM auth_sessions WHERE scope='tenant'") != 0 {
		t.Fatal("rejected WeChat identity created a user, identity fields or tenant session")
	}
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": "valid", "openid": "forged"}), 400)
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": "contains spaces"}), 400)
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID, s.platform.Token, map[string]any{"registration_enabled": false}))
	first := wechatLogin(t, s, "alpha.test", "new_person")
	if s.count(t, "SELECT count(*) FROM users") != 2 || s.count(t, "SELECT count(*) FROM users WHERE open_id<>''") != 1 {
		t.Fatal("WeChat first login was incorrectly blocked by password registration setting")
	}
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID, s.platform.Token, map[string]any{"registration_enabled": true}))
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID, s.platform.Token, map[string]any{"registration_enabled": false}))
	if repeat := wechatLogin(t, s, "alpha.test", "new_person"); repeat.User.ID != first.User.ID {
		t.Fatal("registration closure blocked existing member")
	}
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+alpha.Tenant.ID, s.platform.Token, map[string]any{"status": "disabled"}))
	assertWeChatError(t, s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": "new_person"}), 403, "disabled")
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", first.Token, nil), 403)
	if s.count(t, "SELECT count(*) FROM auth_sessions WHERE scope='tenant'") != 0 {
		t.Fatal("disabled enterprise retained sessions")
	}
}

func TestSaaSWeChatConcurrentFirstLoginAndAtomicRollback(t *testing.T) {
	s := newIntegrationSuite(t)
	alpha := s.createTenant(t, "alpha.test")
	configureWeChat(t, s, func(_ context.Context, c port.WeChatCredentials, _ string) (port.WeChatIdentity, error) {
		return port.WeChatIdentity{AppID: c.AppID, OpenID: "same_verified_person"}, nil
	})
	const workers = 6
	var wg sync.WaitGroup
	results := make(chan apiResult, workers)
	for index := range workers {
		wg.Go(func() {
			results <- s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": fmt.Sprintf("fresh_code_%d", index)})
		})
	}
	wg.Wait()
	close(results)
	var userID string
	for result := range results {
		login := decodeSuccess[loginView](t, result)
		assertLogin(t, login)
		if userID != "" && userID != login.User.ID {
			t.Fatal("concurrent first login created multiple users")
		}
		userID = login.User.ID
	}
	if s.count(t, "SELECT count(*) FROM users WHERE tenant_id=$1", alpha.Tenant.ID) != 2 || s.count(t, "SELECT count(*) FROM users WHERE open_id<>''") != 1 {
		t.Fatal("concurrent first login left duplicate or orphan accounts")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 在会话写入处制造失败，确认包含微信身份字段的用户也随同一事务回滚。
	if _, err := s.pool.Exec(ctx, "CREATE UNIQUE INDEX test_only_one_tenant_session ON auth_sessions ((1)) WHERE scope='tenant'"); err == nil {
		t.Fatal("fixture should conflict with existing sessions")
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM auth_sessions WHERE scope='tenant'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "CREATE UNIQUE INDEX test_only_one_tenant_session ON auth_sessions ((1)) WHERE scope='tenant'"); err != nil {
		t.Fatal(err)
	}
	wechatLogin(t, s, "alpha.test", "fresh_existing")
	configureWeChat(t, s, fixtureIdentity)
	result := s.request(http.MethodPost, "alpha.test", "/api/auth/wechat/login", "", map[string]any{"code": "must_rollback"})
	if result.status < 400 {
		t.Fatal("forced session write failure succeeded")
	}
	if s.count(t, "SELECT count(*) FROM users WHERE tenant_id=$1", alpha.Tenant.ID) != 2 || s.count(t, "SELECT count(*) FROM users WHERE open_id<>''") != 1 {
		t.Fatal("failed session insert left an orphan WeChat user")
	}
}
