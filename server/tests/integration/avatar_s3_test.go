package saas_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	objectstorage "go-server/internal/app/storage"
	storagedriver "go-server/internal/infra/storage"
)

// 真实对象服务必须显式启用，测试不加载 .env，也不使用默认凭据链。
func TestSaaSAvatarS3PersistsAndEnforcesOwnership(t *testing.T) {
	if os.Getenv("TEST_S3_ENABLED") != "1" {
		t.Skip("set TEST_S3_ENABLED=1 and TEST_S3_* explicitly to run real S3 avatar integration")
	}
	if os.Getenv("SAAS_TEST_DATABASE_URL") == "" {
		t.Fatal("real S3 avatar integration requires an explicit disposable PostgreSQL database")
	}
	pathStyle, err := strconv.ParseBool(os.Getenv("TEST_S3_USE_PATH_STYLE"))
	if err != nil {
		t.Fatal("TEST_S3_USE_PATH_STYLE must be explicitly true or false")
	}
	config := storagedriver.Config{Provider: "s3", S3: storagedriver.S3Config{
		Endpoint: os.Getenv("TEST_S3_ENDPOINT"), Region: os.Getenv("TEST_S3_REGION"), Bucket: os.Getenv("TEST_S3_BUCKET"),
		AccessKeyID: os.Getenv("TEST_S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("TEST_S3_SECRET_ACCESS_KEY"), UsePathStyle: pathStyle,
	}}
	store, err := storagedriver.New(config)
	if err != nil {
		t.Fatal("invalid explicit S3 test configuration")
	}
	s := newIntegrationSuite(t)
	s.config.Storage = config
	s.rebuildHandler(t)
	s.createTenant(t, "alpha.test")
	s.createTenant(t, "beta.test")
	admin := s.login(t, "alpha.test", "tenant.admin", adminPassword)
	created := s.createUser(t, "alpha.test", admin.Token, "s3.member", "user")
	member := s.login(t, "alpha.test", created.Username, memberPassword)
	var imageBody bytes.Buffer
	if err := png.Encode(&imageBody, image.NewRGBA(image.Rect(0, 0, 9, 7))); err != nil {
		t.Fatal(err)
	}
	result := avatarRequest(t, s, "alpha.test", member.Token, imageBody.Bytes())
	uploaded := decodeSuccess[struct {
		AvatarURL string `json:"avatar_url"`
	}](t, result)
	prefix := "/api/avatars/" + member.Tenant.ID + "/" + member.User.ID + "/"
	name := strings.TrimPrefix(uploaded.AvatarURL, prefix)
	random, err := hex.DecodeString(strings.TrimSuffix(name, ".png"))
	if !strings.HasPrefix(uploaded.AvatarURL, prefix) || !strings.HasSuffix(name, ".png") || err != nil || len(random) != 16 {
		t.Fatal("S3 upload did not return a stable user-scoped API URL")
	}
	key := strings.TrimPrefix(uploaded.AvatarURL, "/api/")
	// 只回收本次上传返回的随机对象；桶中其他测试与业务对象不在清理范围。
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := store.Delete(ctx, key); err != nil {
			t.Errorf("clean up uploaded test object %s: delete failed", key)
			return
		}
		if _, err := store.Stat(ctx, key); !errors.Is(err, objectstorage.ErrNotFound) {
			t.Errorf("clean up uploaded test object %s: object was not confirmed missing", key)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	metadata, err := store.Stat(ctx, key)
	if err != nil || metadata.Size != int64(imageBody.Len()) || metadata.ContentType != "image/png" {
		t.Fatal("uploaded object was not persisted with its image metadata in the configured S3 service")
	}
	picture := s.request(http.MethodGet, "alpha.test", uploaded.AvatarURL, "", nil)
	if picture.status != http.StatusOK || !bytes.Equal(picture.body, imageBody.Bytes()) {
		t.Fatal("HTTP did not return the bytes persisted in S3")
	}
	rangeRequest := httptest.NewRequest(http.MethodGet, "http://alpha.test"+uploaded.AvatarURL, nil)
	rangeContext, rangeCancel := context.WithTimeout(rangeRequest.Context(), 15*time.Second)
	defer rangeCancel()
	rangeRequest = rangeRequest.WithContext(rangeContext)
	rangeRequest.Header.Set("Range", "bytes=0-7")
	rangeResult := httptest.NewRecorder()
	s.handler.ServeHTTP(rangeResult, rangeRequest)
	if rangeResult.Code != http.StatusPartialContent || !bytes.Equal(rangeResult.Body.Bytes(), imageBody.Bytes()[:8]) || rangeResult.Header().Get("Content-Type") != "image/png" || rangeResult.Header().Get("Content-Range") != "bytes 0-7/"+strconv.Itoa(imageBody.Len()) {
		t.Fatal("S3-backed avatar lost its HTTP image Range contract")
	}
	update := map[string]any{"nickname": "S3 头像资料", "avatar_url": uploaded.AvatarURL}
	saved := decodeSuccess[userView](t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", member.Token, update))
	if saved.AvatarURL != uploaded.AvatarURL || saved.Nickname != "S3 头像资料" {
		t.Fatal("profile did not persist the stable S3-backed avatar URL")
	}
	assertFailure(t, s.request(http.MethodPut, "alpha.test", "/api/account/profile", admin.Token, update), http.StatusBadRequest)
	beta := s.login(t, "beta.test", "tenant.admin", adminPassword)
	assertFailure(t, s.request(http.MethodPut, "beta.test", "/api/account/profile", beta.Token, update), http.StatusBadRequest)
	current := decodeSuccess[loginView](t, s.request(http.MethodGet, "alpha.test", "/api/auth/me", member.Token, nil))
	if current.User.AvatarURL != uploaded.AvatarURL || current.User.Nickname != "S3 头像资料" {
		t.Fatal("subsequent session read lost the persisted S3-backed profile")
	}
	for host, token := range map[string]string{"alpha.test": admin.Token, "beta.test": beta.Token} {
		other := decodeSuccess[loginView](t, s.request(http.MethodGet, host, "/api/auth/me", token, nil))
		if other.User.AvatarURL != "" {
			t.Fatal("rejected binding changed another account profile")
		}
	}
}
