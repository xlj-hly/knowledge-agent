package saas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSaaSProfileDistinguishesMissingNullAndEmpty(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	decodeSuccess[userView](t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", admin.Token, map[string]any{"nickname": "保留昵称", "avatar_url": "https://alpha.test/avatar.png"}))
	for _, body := range []any{nil, []string{}, "text", map[string]any{}, map[string]any{"nickname": nil, "avatar_url": ""}, map[string]any{"nickname": "changed", "avatar_url": nil}, map[string]any{"nickname": "changed"}, map[string]any{"avatar_url": ""}} {
		// nil 接口也必须真正发送 JSON null，不能被请求辅助函数当作无请求体。
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "http://alpha.test/api/account/profile", bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+admin.Token)
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, req)
		assertFailure(t, apiResult{w.Code, w.Body.Bytes()}, http.StatusBadRequest)
	}
	me := decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", admin.Token, nil))
	if me.User.Nickname != "保留昵称" || me.User.AvatarURL != "https://alpha.test/avatar.png" {
		t.Fatalf("rejected input changed persisted profile: %+v", me.User)
	}
	profile := decodeSuccess[userView](t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", admin.Token, map[string]any{"nickname": "", "avatar_url": ""}))
	if profile.Nickname != "" || profile.AvatarURL != "" {
		t.Fatalf("explicit empty profile fields were not cleared: %+v", profile)
	}
	me = decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", admin.Token, nil))
	if me.User.Nickname != "" || me.User.AvatarURL != "" {
		t.Fatalf("complete profile save not persisted: %+v", me.User)
	}
}

func TestSaaSReadsShareTenantLockAndInvalidInputDoesNotWait(t *testing.T) {
	s := newIntegrationSuite(t)
	created := s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	blocker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, "SELECT id FROM company WHERE id=$1 FOR SHARE", created.Tenant.ID); err != nil {
		t.Fatal(err)
	}
	requestWithDeadline := func(method, path, body string) apiResult {
		req := httptest.NewRequest(method, "http://alpha.test"+path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+admin.Token)
		requestCtx, requestCancel := context.WithTimeout(req.Context(), time.Second)
		defer requestCancel()
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, req.WithContext(requestCtx))
		return apiResult{w.Code, w.Body.Bytes()}
	}
	me := decodeSuccess[loginView](t, requestWithDeadline(http.MethodGet, "/api/auth/me", ""))
	if me.User.ID != created.User.ID {
		t.Fatal("read returned wrong account")
	}
	if _, err := blocker.Exec(ctx, "SELECT id FROM company WHERE id=$1 FOR UPDATE", created.Tenant.ID); err != nil {
		t.Fatal(err)
	}
	assertFailure(t, requestWithDeadline(http.MethodPost, "/api/admin/users", `{"username":"new.user","password":"short","role":"user"}`), http.StatusBadRequest)
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, "SELECT count(*) FROM users WHERE username='new.user'"); n != 0 {
		t.Fatal("invalid input created user")
	}
}

func TestSaaSAuthenticationDatabaseFailureIsNotSessionExpiry(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "alpha.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(ctx, "ALTER TABLE users RENAME TO users_unavailable"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.pool.Exec(cleanup, "ALTER TABLE users_unavailable RENAME TO users"); err != nil {
			t.Errorf("restore users table in %s: %v", s.schema, err)
		}
	})
	result := s.request(http.MethodGet, "alpha.test", "/api/auth/me", admin.Token, nil)
	if result.status != http.StatusInternalServerError {
		t.Fatalf("database failure became authentication failure: HTTP %d %s", result.status, result.body)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "internal_error" {
		t.Fatalf("wrong failure category: %s", result.body)
	}
	if bytes.Contains(result.body, []byte("users")) || bytes.Contains(result.body, []byte("42P01")) {
		t.Fatal("database details leaked")
	}
	if n := s.count(t, "SELECT count(*) FROM auth_sessions WHERE scope='tenant'"); n != 1 {
		t.Fatal("database failure revoked valid session")
	}
}
