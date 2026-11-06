package saas_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jackc/pgx/v5/stdlib"
	"go-server/internal/app/saas/port"
	objectstorage "go-server/internal/app/storage"
	"go-server/internal/bootstrap"
	"go-server/internal/infra/database"
	storagedriver "go-server/internal/infra/storage"
	"gorm.io/gorm"
)

const (
	platformPassword = "Platform-pass-2026"
	adminPassword    = "Tenant-admin-2026"
	memberPassword   = "Member-pass-2026"
)

type apiResult struct {
	status int
	body   []byte
}

type userView struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
	Status    string `json:"status"`
}

type tenantView struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Hostname            string `json:"hostname"`
	Status              string `json:"status"`
	RegistrationEnabled bool   `json:"registration_enabled"`
}

type loginView struct {
	Token     string     `json:"token"`
	ExpiresAt time.Time  `json:"expires_at"`
	User      userView   `json:"user"`
	Tenant    tenantView `json:"tenant"`
	Admin     struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"admin"`
}

type tenantCreatedView struct {
	Tenant tenantView `json:"tenant"`
	User   userView   `json:"user"`
}

type integrationSuite struct {
	db               *gorm.DB
	pool             *pgxpool.Pool
	handler          http.Handler
	platform         loginView
	schema           string
	config           bootstrap.Config
	objectStorage    objectstorage.Store
	wechatProvider   port.WeChatProvider
	wechatConfigured bool
}

// newIntegrationSuite only writes to its own random schema in an explicitly
// selected test database. It never falls back to the application's DATABASE_URL.
func newIntegrationSuite(t *testing.T) *integrationSuite {
	t.Helper()
	databaseURL := os.Getenv("SAAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set SAAS_TEST_DATABASE_URL to enable real PostgreSQL API integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		adminPool.Close()
		t.Fatalf("generate test schema name: %v", err)
	}
	schema := "saas_test_" + hex.EncodeToString(suffix[:])
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("clean up test schema %s: %v", schema, err)
		}
		adminPool.Close()
	})
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database configuration: %v", err)
	}
	poolConfig.MaxConns = 8
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	poolConfig.ConnConfig.RuntimeParams["application_name"] = schema
	pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("connect isolated schema: %v", err)
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate SaaS schema file from integration test source")
	}
	schemaPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "db", "saas.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read SaaS schema: %v", err)
	}
	// 正式建表目标为 public，测试只移除此设置，让完整 SQL 在本次隔离 schema 内执行。
	sql := strings.ReplaceAll(string(schemaSQL), "SET LOCAL search_path = public;", "")
	if _, err := pool.Exec(ctx, sql); err != nil {
		t.Fatalf("create SaaS tables in %s: %v", schema, err)
	}
	var actualSchema string
	if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&actualSchema); err != nil {
		t.Fatalf("verify schema isolation: %v", err)
	}
	if actualSchema != schema {
		t.Fatalf("SaaS tables escaped test schema: got %q, want %q", actualSchema, schema)
	}
	sqlPool := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() {
		if err := sqlPool.Close(); err != nil {
			t.Errorf("close GORM pool: %v", err)
		}
	})
	db, err := database.Wrap(sqlPool)
	if err != nil {
		t.Fatalf("open GORM database: %v", err)
	}
	if err := database.Ready(ctx, db); err != nil {
		t.Fatalf("SaaS schema is not ready: %v", err)
	}
	if err := bootstrap.Bootstrap(ctx, db, "platform.root", platformPassword); err != nil {
		t.Fatalf("bootstrap platform administrator: %v", err)
	}
	config := bootstrap.Config{SessionTTL: 7 * 24 * time.Hour, AllowedOrigins: []string{"http://localhost:5174", "http://platform.localhost:5174", "https://app.alpha.test"}, PlatformHosts: []string{"platform.test", "platform.localhost"}, Storage: storagedriver.Config{Provider: "filesystem", FilesystemRoot: t.TempDir()}}
	suite := &integrationSuite{
		db:     db,
		pool:   pool,
		schema: schema,
		config: config,
	}
	suite.rebuildHandler(t)
	suite.platform = decodeSuccess[loginView](t, suite.request(http.MethodPost, "platform.test", "/api/platform/auth/login", "", map[string]any{
		"username": "platform.root", "password": platformPassword,
	}))
	assertLogin(t, suite.platform)
	return suite
}

func (s *integrationSuite) rebuildHandler(t *testing.T) {
	t.Helper()
	var options []bootstrap.Option
	if s.wechatProvider != nil {
		options = append(options, bootstrap.WithWeChatProvider(s.wechatProvider))
	}
	if s.objectStorage != nil {
		options = append(options, bootstrap.WithObjectStorage(s.objectStorage))
	}
	application, err := bootstrap.New(s.db, s.config, options...)
	if err != nil {
		t.Fatalf("assemble SaaS application: %v", err)
	}
	s.handler = application.Handler
}

func (s *integrationSuite) request(method, host, path, token string, body any) apiResult {
	if host == "platform.test" {
		host = "platform.localhost"
	}
	var requestBody []byte
	if body != nil {
		var err error
		requestBody, err = json.Marshal(body)
		if err != nil {
			panic(fmt.Sprintf("marshal test request: %v", err))
		}
	}
	req := httptest.NewRequest(method, "http://"+host+path, bytes.NewReader(requestBody))
	ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	req.Host = host
	// A unique peer per request keeps rate limiting from conflating independent
	// acceptance scenarios while still executing the real router and middleware.
	var peer [4]byte
	if _, err := rand.Read(peer[:]); err != nil {
		panic(err)
	}
	req.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:12345", peer[0], peer[1], peer[2])
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, req)
	return apiResult{status: recorder.Code, body: recorder.Body.Bytes()}
}

func decodeSuccess[T any](t *testing.T, result apiResult) T {
	t.Helper()
	if result.status < 200 || result.status >= 300 {
		t.Fatalf("expected successful API response, got HTTP %d: %s", result.status, result.body)
	}
	assertNoPasswordHash(t, result)
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(result.body, &envelope); err != nil {
		t.Fatalf("decode success envelope: %v; body=%s", err, result.body)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" || len(envelope.Error) != 0 {
		t.Fatalf("invalid success envelope: %s", result.body)
	}
	var data T
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode response data: %v; body=%s", err, result.body)
	}
	return data
}

func assertFailure(t *testing.T, result apiResult, allowedStatuses ...int) {
	t.Helper()
	if result.status < 400 || result.status >= 500 {
		t.Fatalf("expected rejected client request, got HTTP %d: %s", result.status, result.body)
	}
	if len(allowedStatuses) > 0 {
		matched := false
		for _, status := range allowedStatuses {
			matched = matched || result.status == status
		}
		if !matched {
			t.Fatalf("expected HTTP %v, got HTTP %d: %s", allowedStatuses, result.status, result.body)
		}
	}
	assertNoPasswordHash(t, result)
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.body, &envelope); err != nil {
		t.Fatalf("decode error envelope: %v; body=%s", err, result.body)
	}
	if envelope.Error.Code == "" || envelope.Error.Message == "" || len(envelope.Data) != 0 {
		t.Fatalf("invalid error envelope: %s", result.body)
	}
}

func assertNoPasswordHash(t *testing.T, result apiResult) {
	t.Helper()
	if bytes.Contains(result.body, []byte("password_hash")) {
		t.Fatalf("API response exposes password hash: HTTP %d", result.status)
	}
}

func assertLogin(t *testing.T, login loginView) {
	t.Helper()
	if len(login.Token) < 32 || !login.ExpiresAt.After(time.Now()) {
		t.Fatalf("login did not issue an opaque token and future expiry: token_length=%d expiry=%s", len(login.Token), login.ExpiresAt)
	}
}

func (s *integrationSuite) createTenant(t *testing.T, host string) tenantCreatedView {
	t.Helper()
	created := decodeSuccess[tenantCreatedView](t, s.request(http.MethodPost, "platform.test", "/api/platform/tenants", s.platform.Token, map[string]any{
		"name": host, "hostname": host, "admin_username": "tenant.admin", "admin_password": adminPassword,
	}))
	if created.Tenant.ID == "" || created.Tenant.Hostname != host || created.User.ID == "" || created.User.Role != "admin" || created.User.TenantID != created.Tenant.ID {
		t.Fatalf("tenant creation did not return its first administrator: %+v", created)
	}
	if s.wechatConfigured {
		saveFixtureWeChat(t, s, created.Tenant.ID, host)
	}
	return created
}

func (s *integrationSuite) login(t *testing.T, host, username, password string) loginView {
	t.Helper()
	login := decodeSuccess[loginView](t, s.request(http.MethodPost, host, "/api/auth/login", "", map[string]any{
		"username": username, "password": password,
	}))
	assertLogin(t, login)
	return login
}

func (s *integrationSuite) createUser(t *testing.T, host, adminToken, username, role string) userView {
	t.Helper()
	user := decodeSuccess[userView](t, s.request(http.MethodPost, host, "/api/admin/users", adminToken, map[string]any{
		"username": username, "password": memberPassword, "role": role,
	}))
	if user.Username != username || user.Role != role || user.Status != "active" || user.ID == "" {
		t.Fatalf("created account has wrong identity or permissions: %+v", user)
	}
	return user
}

func (s *integrationSuite) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("query persisted acceptance result: %v", err)
	}
	return count
}

func awaitDatabaseCondition(t *testing.T, pool *pgxpool.Pool, description, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var ready bool
		if err := pool.QueryRow(ctx, query, args...).Scan(&ready); err != nil {
			t.Fatalf("wait for %s: %v", description, err)
		}
		if ready {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", description)
		case <-ticker.C:
		}
	}
}

func TestSaaSPlatformCreatesIsolatedTenantsAtomically(t *testing.T) {
	s := newIntegrationSuite(t)
	platformMe := decodeSuccess[struct {
		Admin struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"admin"`
	}](t, s.request(http.MethodGet, "platform.test", "/api/platform/auth/me", s.platform.Token, nil))
	if platformMe.Admin.ID == "" || platformMe.Admin.Username != "platform.root" {
		t.Fatalf("platform bootstrap identity not readable: %+v", platformMe)
	}
	alpha := s.createTenant(t, "alpha.test")
	beta := s.createTenant(t, "beta.test")
	if alpha.Tenant.ID == beta.Tenant.ID || alpha.User.ID == beta.User.ID {
		t.Fatal("distinct tenants share an account or tenant identity")
	}
	alphaLogin := s.login(t, "alpha.test:8080", "TENANT.ADMIN", adminPassword)
	betaLogin := s.login(t, "beta.test", "tenant.admin", adminPassword)
	if alphaLogin.User.ID != alpha.User.ID || betaLogin.User.ID != beta.User.ID || alphaLogin.User.Username != "tenant.admin" {
		t.Fatal("same login name was not independently resolved and normalized within each tenant")
	}
	tokenHash := sha256.Sum256([]byte(alphaLogin.Token))
	if got := s.count(t, "SELECT count(*) FROM auth_sessions WHERE token_hash = $1 AND scope = 'tenant' AND subject_id = $2 AND tenant_id = $3", hex.EncodeToString(tokenHash[:]), alpha.User.ID, alpha.Tenant.ID); got != 1 {
		t.Fatal("issued enterprise session was not persisted as a SHA256 hash with its scope")
	}
	if got := s.count(t, "SELECT count(*) FROM auth_sessions WHERE token_hash = $1", alphaLogin.Token); got != 0 {
		t.Fatal("session persisted its raw bearer token")
	}
	resolved := decodeSuccess[tenantView](t, s.request(http.MethodGet, "alpha.test:8080", "/api/tenant", "", nil))
	if resolved.ID != alpha.Tenant.ID {
		t.Fatalf("Host with port resolved wrong tenant: %+v", resolved)
	}
	list := decodeSuccess[struct {
		Items []tenantView `json:"items"`
		Total int          `json:"total"`
	}](t, s.request(http.MethodGet, "platform.test", "/api/platform/tenants?q=alpha&page=1&page_size=20", s.platform.Token, nil))
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != alpha.Tenant.ID {
		t.Fatalf("platform tenant search does not return persisted tenant: %+v", list)
	}
	assertFailure(t, s.request(http.MethodPost, "platform.test", "/api/platform/tenants", s.platform.Token, map[string]any{
		"name": "duplicate", "hostname": "ALPHA.TEST", "admin_username": "another.admin", "admin_password": adminPassword,
	}))
	if got := s.count(t, "SELECT count(*) FROM company"); got != 2 {
		t.Fatalf("duplicate hostname created extra tenant: %d", got)
	}
	// Force the first-admin insert to fail after a valid tenant insert. The
	// temporary index exists only inside this test schema and verifies rollback.
	s.createUser(t, "alpha.test", alphaLogin.Token, "reserved.admin", "user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(ctx, "CREATE UNIQUE INDEX test_one_reserved_admin ON users ((1)) WHERE username = 'reserved.admin'"); err != nil {
		t.Fatalf("install transaction failure fixture: %v", err)
	}
	result := s.request(http.MethodPost, "platform.test", "/api/platform/tenants", s.platform.Token, map[string]any{
		"name": "must rollback", "hostname": "rollback.test", "admin_username": "reserved.admin", "admin_password": adminPassword,
	})
	if result.status < 400 {
		t.Fatalf("forced administrator insert failure succeeded: HTTP %d", result.status)
	}
	if got := s.count(t, "SELECT count(*) FROM company WHERE hostname = $1", "rollback.test"); got != 0 {
		t.Fatal("failed first administrator creation left an orphan tenant")
	}
}

func TestSaaSRejectsTenantAndRoleEscalationWithoutWrites(t *testing.T) {
	s := newIntegrationSuite(t)
	alpha := s.createTenant(t, "alpha.test")
	s.createTenant(t, "beta.test")
	alphaAdmin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	betaAdmin := s.login(t, "beta.test", "tenant.admin", adminPassword)
	alphaMember := s.createUser(t, "alpha.test", alphaAdmin.Token, "ordinary.user", "user")
	betaMember := s.createUser(t, "beta.test", betaAdmin.Token, "ordinary.user", "user")
	member := s.login(t, "alpha.test", "ordinary.user", memberPassword)
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/admin/users", member.Token, nil), http.StatusForbidden)
	before := s.count(t, "SELECT count(*) FROM users")
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/admin/users", member.Token, map[string]any{
		"username": "illicit.user", "password": memberPassword, "role": "admin",
	}), http.StatusForbidden)
	assertFailure(t, s.request(http.MethodPost, "beta.test", "/api/admin/users", alphaAdmin.Token, map[string]any{
		"username": "cross.tenant", "password": memberPassword, "role": "admin",
	}), http.StatusUnauthorized, http.StatusForbidden)
	assertFailure(t, s.request(http.MethodPatch, "alpha.test", "/api/admin/users/"+betaMember.ID, alphaAdmin.Token, map[string]any{
		"role": "admin", "status": "disabled",
	}), http.StatusForbidden, http.StatusNotFound)
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/admin/users/"+betaMember.ID+"/reset-password", alphaAdmin.Token, map[string]any{
		"new_password": "Cross-tenant-new-2026",
	}), http.StatusForbidden, http.StatusNotFound)
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", s.platform.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	assertFailure(t, s.request(http.MethodGet, "platform.test", "/api/platform/tenants", alphaAdmin.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	if got := s.count(t, "SELECT count(*) FROM users"); got != before {
		t.Fatalf("rejected access created accounts: before=%d after=%d", before, got)
	}
	if got := s.count(t, "SELECT count(*) FROM users WHERE id = $1 AND tenant_id = $2 AND role = 'user' AND status = 'active'", alphaMember.ID, alpha.Tenant.ID); got != 1 {
		t.Fatal("rejected access changed original member")
	}
	if got := s.count(t, "SELECT count(*) FROM users WHERE id = $1 AND role = 'user' AND status = 'active'", betaMember.ID); got != 1 {
		t.Fatal("cross-tenant update changed target account")
	}
	s.login(t, "beta.test", "ordinary.user", memberPassword)
}

func TestSaaSRegistrationHasNoPrivilegeFieldsAndHonorsClosure(t *testing.T) {
	s := newIntegrationSuite(t)
	created := s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "alpha.test", "/api/admin/tenant", admin.Token, map[string]any{"registration_enabled": true}))
	before := s.count(t, "SELECT count(*) FROM users")
	for _, extra := range []map[string]any{{"role": "admin"}, {"tenant_id": created.Tenant.ID}, {"status": "active"}} {
		body := map[string]any{"username": "injected.user", "password": memberPassword}
		for key, value := range extra {
			body[key] = value
		}
		assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/register", "", body), http.StatusBadRequest)
	}
	if got := s.count(t, "SELECT count(*) FROM users"); got != before {
		t.Fatal("rejected registration with server-owned fields created an account")
	}
	registered := decodeSuccess[loginView](t, s.request(http.MethodPost, "alpha.test", "/api/auth/register", "", map[string]any{
		"username": "New.Member", "password": memberPassword, "nickname": "新成员",
	}))
	assertLogin(t, registered)
	if registered.User.Role != "user" || registered.User.Username != "new.member" || registered.User.TenantID != created.Tenant.ID || registered.User.Nickname != "新成员" {
		t.Fatalf("registration did not create an ordinary account in current tenant: %+v", registered.User)
	}
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/register", "", map[string]any{"username": "NEW.MEMBER", "password": memberPassword}))
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "alpha.test", "/api/admin/tenant", admin.Token, map[string]any{"registration_enabled": false}))
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/register", "", map[string]any{"username": "closed.user", "password": memberPassword}))
	if got := s.count(t, "SELECT count(*) FROM users WHERE username = 'closed.user'"); got != 0 {
		t.Fatal("closed registration still created an account")
	}
	for _, body := range []map[string]any{
		{"username": "ab", "password": memberPassword},
		{"username": "bad space", "password": memberPassword},
		{"username": "valid.user", "password": "123456789"},
		{"username": "valid.user", "password": strings.Repeat("a", 73)},
		{"username": "valid.user", "password": strings.Repeat("密", 25)},
	} {
		body["role"] = "user"
		assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/admin/users", admin.Token, body), http.StatusBadRequest)
	}
	if got := s.count(t, "SELECT count(*) FROM users"); got != before+1 {
		t.Fatalf("invalid account creation changed user count: got %d want %d", got, before+1)
	}
	for _, fixture := range []struct{ username, password string }{{"minimum.user", "1234567890"}, {"maximum.user", strings.Repeat("a", 72)}, {"unicode.user", "测试密码"}} {
		decodeSuccess[userView](t, s.request(http.MethodPost, "alpha.test", "/api/admin/users", admin.Token, map[string]any{"username": fixture.username, "password": fixture.password, "role": "user"}))
		s.login(t, "alpha.test", fixture.username, fixture.password)
	}
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/admin/users", admin.Token, map[string]any{"username": "MAXIMUM.USER", "password": memberPassword, "role": "user"}))
	if got := s.count(t, "SELECT count(*) FROM users WHERE username = 'maximum.user'"); got != 1 {
		t.Fatal("duplicate normalized username changed the account population")
	}
}

func TestSaaSProfilePasswordRevokesSessionsAndLogout(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	s.createUser(t, "alpha.test", admin.Token, "profile.user", "user")
	first := s.login(t, "alpha.test", "profile.user", memberPassword)
	second := s.login(t, "alpha.test", "profile.user", memberPassword)
	profile := decodeSuccess[userView](t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", first.Token, map[string]any{
		"nickname": "小林", "avatar_url": "https://example.test/avatar.png",
	}))
	if profile.Nickname != "小林" || profile.AvatarURL != "https://example.test/avatar.png" {
		t.Fatalf("profile update returned wrong values: %+v", profile)
	}
	me := decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", second.Token, nil))
	if me.User.Nickname != "小林" || me.User.AvatarURL != "https://example.test/avatar.png" {
		t.Fatalf("profile update was not persisted: %+v", me.User)
	}
	assertFailure(t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", first.Token, map[string]any{"nickname": "小林", "avatar_url": "https://example.test/avatar.png", "role": "admin"}), http.StatusBadRequest)
	assertFailure(t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", first.Token, map[string]any{"nickname": "小林", "avatar_url": "javascript:alert(1)"}), http.StatusBadRequest)
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/account/password", first.Token, map[string]any{"current_password": "wrong-password", "new_password": "New-member-pass-2026"}))
	decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", first.Token, nil))
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "alpha.test", "/api/account/password", first.Token, map[string]any{"current_password": memberPassword, "new_password": "New-member-pass-2026"}))
	for _, token := range []string{first.Token, second.Token} {
		assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", token, nil), http.StatusUnauthorized, http.StatusForbidden)
	}
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/login", "", map[string]any{"username": "profile.user", "password": memberPassword}), http.StatusUnauthorized, http.StatusForbidden)
	fresh := s.login(t, "alpha.test", "profile.user", "New-member-pass-2026")
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "alpha.test", "/api/auth/logout", fresh.Token, nil))
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", fresh.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", admin.Token, nil))
}

func TestSaaSDisabledAccountsAndTenantsCannotReviveOldTokens(t *testing.T) {
	s := newIntegrationSuite(t)
	created := s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	user := s.createUser(t, "alpha.test", admin.Token, "disabled.user", "user")
	member := s.login(t, "alpha.test", user.Username, memberPassword)
	decodeSuccess[userView](t, s.request(http.MethodPatch, "alpha.test", "/api/admin/users/"+user.ID, admin.Token, map[string]any{"status": "disabled"}))
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", member.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	decodeSuccess[userView](t, s.request(http.MethodPatch, "alpha.test", "/api/admin/users/"+user.ID, admin.Token, map[string]any{"status": "active"}))
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", member.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	member = s.login(t, "alpha.test", user.Username, memberPassword)
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+created.Tenant.ID, s.platform.Token, map[string]any{"status": "disabled"}))
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", member.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/login", "", map[string]any{"username": user.Username, "password": memberPassword}))
	decodeSuccess[tenantView](t, s.request(http.MethodPatch, "platform.test", "/api/platform/tenants/"+created.Tenant.ID, s.platform.Token, map[string]any{"status": "active"}))
	for _, token := range []string{member.Token, admin.Token} {
		assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", token, nil), http.StatusUnauthorized, http.StatusForbidden)
	}
	s.login(t, "alpha.test", user.Username, memberPassword)
	s.login(t, "alpha.test", "tenant.admin", adminPassword)
}

func TestSaaSLastActiveAdministratorSurvivesConcurrentDemotion(t *testing.T) {
	s := newIntegrationSuite(t)
	created := s.createTenant(t, "alpha.test")
	first := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	for _, body := range []map[string]any{{"role": "user"}, {"status": "disabled"}} {
		assertFailure(t, s.request(http.MethodPatch, "alpha.test", "/api/admin/users/"+created.User.ID, first.Token, body))
	}
	if got := s.count(t, "SELECT count(*) FROM users WHERE tenant_id = $1 AND role = 'admin' AND status = 'active'", created.Tenant.ID); got != 1 {
		t.Fatal("rejected last-administrator edit changed effective administrator count")
	}
	secondUser := s.createUser(t, "alpha.test", first.Token, "second.admin", "admin")
	second := s.login(t, "alpha.test", secondUser.Username, memberPassword)
	start := make(chan struct{})
	results := make(chan apiResult, 2)
	for _, attempt := range []struct{ id, token string }{{created.User.ID, first.Token}, {secondUser.ID, second.Token}} {
		go func(id, token string) {
			<-start
			results <- s.request(http.MethodPatch, "alpha.test", "/api/admin/users/"+id, token, map[string]any{"role": "user"})
		}(attempt.id, attempt.token)
	}
	close(start)
	succeeded := 0
	for range 2 {
		result := <-results
		if result.status >= 200 && result.status < 300 {
			decodeSuccess[userView](t, result)
			succeeded++
		} else {
			assertFailure(t, result)
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent administrator demotion: got %d successes, want 1", succeeded)
	}
	if got := s.count(t, "SELECT count(*) FROM users WHERE tenant_id = $1 AND role = 'admin' AND status = 'active'", created.Tenant.ID); got != 1 {
		t.Fatalf("concurrent changes left %d active administrators, want 1", got)
	}
}

func TestSaaSPasswordResetsAndPlatformLogoutRevokeSessions(t *testing.T) {
	s := newIntegrationSuite(t)
	created := s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	user := s.createUser(t, "alpha.test", admin.Token, "reset.user", "user")
	member := s.login(t, "alpha.test", user.Username, memberPassword)
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "alpha.test", "/api/admin/users/"+user.ID+"/reset-password", admin.Token, map[string]any{"new_password": "Reset-member-pass-2026"}))
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", member.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	assertFailure(t, s.request(http.MethodPost, "alpha.test", "/api/auth/login", "", map[string]any{"username": user.Username, "password": memberPassword}), http.StatusUnauthorized, http.StatusForbidden)
	s.login(t, "alpha.test", user.Username, "Reset-member-pass-2026")
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "platform.test", "/api/platform/tenants/"+created.Tenant.ID+"/reset-admin-password", s.platform.Token, map[string]any{"username": "tenant.admin", "new_password": "Reset-admin-pass-2026"}))
	assertFailure(t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", admin.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
	s.login(t, "alpha.test", "tenant.admin", "Reset-admin-pass-2026")
	secondPlatform := decodeSuccess[loginView](t, s.request(http.MethodPost, "platform.test", "/api/platform/auth/login", "", map[string]any{"username": "platform.root", "password": platformPassword}))
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "platform.test", "/api/platform/account/password", s.platform.Token, map[string]any{"current_password": platformPassword, "new_password": "New-platform-pass-2026"}))
	for _, token := range []string{s.platform.Token, secondPlatform.Token} {
		assertFailure(t, s.request(http.MethodGet, "platform.test", "/api/platform/auth/me", token, nil), http.StatusUnauthorized, http.StatusForbidden)
	}
	freshPlatform := decodeSuccess[loginView](t, s.request(http.MethodPost, "platform.test", "/api/platform/auth/login", "", map[string]any{"username": "platform.root", "password": "New-platform-pass-2026"}))
	decodeSuccess[map[string]bool](t, s.request(http.MethodPost, "platform.test", "/api/platform/auth/logout", freshPlatform.Token, nil))
	assertFailure(t, s.request(http.MethodGet, "platform.test", "/api/platform/auth/me", freshPlatform.Token, nil), http.StatusUnauthorized, http.StatusForbidden)
}

func TestSaaSSessionExpiresWhileAuthenticationWaitsForTenantLock(t *testing.T) {
	s := newIntegrationSuite(t)
	created := s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	assertSessionExpiresWhileLocked(t, s, "alpha.test", "/api/auth/me", admin.Token,
		"SELECT id FROM company WHERE id=$1 FOR UPDATE", created.Tenant.ID, "%FROM \"company\"%")
}

func TestSaaSPlatformSessionExpiresWhileAuthenticationWaitsForAccountLock(t *testing.T) {
	s := newIntegrationSuite(t)
	assertSessionExpiresWhileLocked(t, s, "platform.test", "/api/platform/auth/me", s.platform.Token,
		"SELECT id FROM super_admin WHERE id=$1 FOR UPDATE", s.platform.Admin.ID, "%FROM \"super_admin\"%")
}

func assertSessionExpiresWhileLocked(t *testing.T, s *integrationSuite, host, path, token, lockQuery, subjectID, blockedQuery string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	blocker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin controlled lifecycle lock: %v", err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, lockQuery, subjectID); err != nil {
		t.Fatalf("hold lifecycle lock before authentication: %v", err)
	}
	tokenHash := sha256.Sum256([]byte(token))
	digest := hex.EncodeToString(tokenHash[:])
	if _, err := s.pool.Exec(ctx, "UPDATE auth_sessions SET expires_at=clock_timestamp()+interval '2 seconds' WHERE token_hash=$1", digest); err != nil {
		t.Fatalf("set controlled session expiration: %v", err)
	}
	result := make(chan apiResult, 1)
	go func() { result <- s.request(http.MethodGet, host, path, token, nil) }()
	awaitDatabaseCondition(t, s.pool, "authentication transaction to wait on a lifecycle lock before expiry", `
		SELECT EXISTS (
		    SELECT 1 FROM pg_stat_activity a CROSS JOIN auth_sessions s
		    WHERE a.application_name=$1 AND a.wait_event_type='Lock'
		      AND a.query LIKE $3 AND a.xact_start<s.expires_at
		      AND s.token_hash=$2
		)`, s.schema, digest, blockedQuery)
	awaitDatabaseCondition(t, s.pool, "the waiting session to expire", "SELECT clock_timestamp()>=expires_at FROM auth_sessions WHERE token_hash=$1", digest)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release controlled lifecycle lock: %v", err)
	}
	select {
	case response := <-result:
		assertFailure(t, response, http.StatusUnauthorized, http.StatusForbidden)
	case <-ctx.Done():
		t.Fatal("authentication did not finish after releasing the lifecycle lock")
	}
}
