package saas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	objectstorage "go-server/internal/app/storage"
)

func avatarRequest(t *testing.T, s *integrationSuite, host, token string, body []byte) apiResult {
	t.Helper()
	var wire bytes.Buffer
	writer := multipart.NewWriter(&wire)
	part, err := writer.CreateFormFile("file", "avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/account/avatar", &wire)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.handler.ServeHTTP(w, req)
	return apiResult{status: w.Code, body: w.Body.Bytes()}
}

type failingObjectStorage struct{}

func (failingObjectStorage) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("test object storage unavailable")
}

func (failingObjectStorage) Get(context.Context, string) (objectstorage.Object, error) {
	return objectstorage.Object{}, errors.New("test object storage unavailable")
}

func (failingObjectStorage) Stat(context.Context, string) (objectstorage.Metadata, error) {
	return objectstorage.Metadata{}, errors.New("test object storage unavailable")
}

func (failingObjectStorage) Delete(context.Context, string) error {
	return errors.New("test object storage unavailable")
}

func assertStorageFailure(t *testing.T, result apiResult) {
	t.Helper()
	if result.status != http.StatusInternalServerError {
		t.Fatalf("object storage failure became HTTP %d: %s", result.status, result.body)
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
		t.Fatalf("object storage failure lost its internal error category: %s", result.body)
	}
	if bytes.Contains(result.body, []byte("test object storage unavailable")) {
		t.Fatal("object storage failure exposed infrastructure details")
	}
}

func TestSaaSAvatarObjectStorageFailuresRemainInternalErrors(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "alpha.test")
	member := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	badResource := s.request(http.MethodGet, "alpha.test", "/api/avatars/1/2/not-a-random-name.png", "", nil)
	if badResource.status != 404 {
		t.Fatalf("invalid avatar path became HTTP %d", badResource.status)
	}
	s.objectStorage = failingObjectStorage{}
	s.rebuildHandler(t)
	var body bytes.Buffer
	if err := png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	assertStorageFailure(t, avatarRequest(t, s, "alpha.test", member.Token, body.Bytes()))
	path := "/api/avatars/" + member.Tenant.ID + "/" + member.User.ID + "/" + strings.Repeat("a", 32) + ".png"
	assertStorageFailure(t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", member.Token, map[string]any{"nickname": "a", "avatar_url": path}))
	assertStorageFailure(t, s.request(http.MethodGet, "alpha.test", path, "", nil))
	current := decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", member.Token, nil))
	if current.User.AvatarURL != "" || current.User.Nickname != member.User.Nickname {
		t.Fatal("failed ownership check mutated profile")
	}
}

func TestSaaSAvatarUploadPersistsAndEnforcesOwnership(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "alpha.test")
	s.createTenant(t, "beta.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	other := s.createUser(t, "alpha.test", admin.Token, "other.member", "user")
	member := s.login(t, "alpha.test", other.Username, memberPassword)
	var imageBody bytes.Buffer
	if err := png.Encode(&imageBody, image.NewRGBA(image.Rect(0, 0, 12, 12))); err != nil {
		t.Fatal(err)
	}
	assertFailure(t, avatarRequest(t, s, "alpha.test", "", imageBody.Bytes()), 401)
	assertFailure(t, avatarRequest(t, s, "alpha.test", member.Token, []byte("pretend image")), 400)
	assertFailure(t, avatarRequest(t, s, "alpha.test", member.Token, bytes.Repeat([]byte("x"), 2*1024*1024+1)), 400)
	uploaded := decodeSuccess[struct {
		AvatarURL string `json:"avatar_url"`
	}](t, avatarRequest(t, s, "alpha.test", member.Token, imageBody.Bytes()))
	if !strings.HasPrefix(uploaded.AvatarURL, "/api/avatars/"+member.Tenant.ID+"/"+member.User.ID+"/") {
		t.Fatal("avatar path not scoped to authenticated user")
	}
	picture := s.request(http.MethodGet, "alpha.test", uploaded.AvatarURL, "", nil)
	if picture.status != 200 || !bytes.Equal(picture.body, imageBody.Bytes()) {
		t.Fatal("persisted avatar could not be read")
	}
	update := map[string]any{"nickname": "微信昵称", "avatar_url": uploaded.AvatarURL}
	saved := decodeSuccess[userView](t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", member.Token, update))
	if saved.AvatarURL != uploaded.AvatarURL {
		t.Fatal("profile did not retain uploaded path")
	}
	assertFailure(t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", admin.Token, update), 400)
	missing := strings.TrimSuffix(uploaded.AvatarURL, ".png") + "0.png"
	assertFailure(t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", member.Token, map[string]any{"nickname": "a", "avatar_url": missing}), 400)
	beta := s.login(t, "beta.test", "tenant.admin", adminPassword)
	assertFailure(t, s.request(http.MethodPut, "beta.test", "/api/account/profile", beta.Token, update), 400)
	current := decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", member.Token, nil))
	if current.User.Nickname != "微信昵称" || current.User.AvatarURL != uploaded.AvatarURL {
		t.Fatal("avatar profile changes were not persisted")
	}
}
