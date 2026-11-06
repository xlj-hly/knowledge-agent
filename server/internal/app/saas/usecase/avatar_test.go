package usecase

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"

	"go-server/internal/app/saas/dto"
	"go-server/internal/app/storage"
	"go-server/internal/domain/saas"
)

type avatarRepository struct {
	domain.Store
	valid         bool
	user          domain.User
	inTransaction bool
}

func (r *avatarRepository) TenantByHost(string, domain.Lock) (domain.Tenant, error) {
	return domain.Tenant{ID: "1", Hostname: "alpha.test", Status: "active"}, nil
}
func (r *avatarRepository) SessionSubject(string, string, string) (string, error) {
	if !r.valid {
		return "", domain.Unauthorized()
	}
	return r.user.ID, nil
}
func (r *avatarRepository) UserByID(string, string, domain.Lock) (domain.Account, error) {
	return domain.Account{User: r.user}, nil
}
func (r *avatarRepository) UpdateUser(_, _ string, change domain.UserChanges) (domain.User, error) {
	r.user.Nickname, r.user.AvatarURL = *change.Nickname, *change.AvatarURL
	return r.user, nil
}

type avatarPersistence struct{ repository *avatarRepository }

func (p avatarPersistence) WithContext(context.Context) domain.Store { return p.repository }
func (p avatarPersistence) Transaction(ctx context.Context, fn func(domain.Store) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.repository.inTransaction = true
	defer func() { p.repository.inTransaction = false }()
	return fn(p.repository)
}
func (avatarPersistence) Ping(context.Context) error            { return nil }
func (avatarPersistence) CleanupSessions(context.Context) error { return nil }

type avatarObjects struct {
	values     map[string][]byte
	afterPut   func()
	putErr     error
	statErr    error
	deleteErr  error
	getKey     string
	beforeStat func()
}

func (s *avatarObjects) Put(ctx context.Context, key string, body io.Reader, _ int64, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	value, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.values[key] = value
	if s.afterPut != nil {
		s.afterPut()
	}
	return s.putErr
}
func (s *avatarObjects) Get(ctx context.Context, key string) (storage.Object, error) {
	s.getKey = key
	if err := ctx.Err(); err != nil {
		return storage.Object{}, err
	}
	value, ok := s.values[key]
	if !ok {
		return storage.Object{}, storage.ErrNotFound
	}
	return storage.Object{Body: io.NopCloser(bytes.NewReader(value)), Metadata: storage.Metadata{Size: int64(len(value)), ContentType: "image/png"}}, nil
}
func (s *avatarObjects) Stat(ctx context.Context, key string) (storage.Metadata, error) {
	if s.beforeStat != nil {
		s.beforeStat()
	}
	if err := ctx.Err(); err != nil {
		return storage.Metadata{}, err
	}
	if s.statErr != nil {
		return storage.Metadata{}, s.statErr
	}
	value, ok := s.values[key]
	if !ok {
		return storage.Metadata{}, storage.ErrNotFound
	}
	return storage.Metadata{Size: int64(len(value)), ContentType: "image/png"}, nil
}

func TestProfileChecksRemoteObjectOutsideTransactionAndRevalidatesSession(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "save profile", true: "session revoked during stat"}[revoke], func(t *testing.T) {
			s, repo, objects, _ := avatarSetup(t)
			name := strings.Repeat("a", 32) + ".png"
			objects.values["avatars/1/2/"+name] = []byte("image")
			called := false
			objects.beforeStat = func() {
				called = true
				if repo.inTransaction {
					t.Fatal("远程对象查询持有数据库事务和业务锁")
				}
				if revoke {
					repo.valid = false
				}
			}
			nickname, path := "updated", "/api/avatars/1/2/"+name
			_, err := s.Accounts.Profile(context.Background(), dto.TenantIdentity{Hostname: "alpha.test", Token: "token"}, dto.ProfileInput{Nickname: &nickname, AvatarURL: &path})
			if !called {
				t.Fatal("没有校验对象存在性")
			}
			if revoke {
				var failure *domain.Error
				if !errors.As(err, &failure) || failure.Code != "unauthorized" || repo.user.AvatarURL != "" {
					t.Fatalf("会话撤销后提交了资料：%v", err)
				}
			} else if err != nil || repo.user.AvatarURL != path {
				t.Fatalf("有效资料未保存：%v", err)
			}
		})
	}
}
func (s *avatarObjects) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.values, key)
	return nil
}

func TestAvatarCleanupFailureRetainsAuthenticationAndDiagnostic(t *testing.T) {
	s, repo, objects, body := avatarSetup(t)
	objects.afterPut = func() { repo.valid = false }
	objects.deleteErr = errors.New("storage cleanup failure")
	_, err := s.Accounts.UploadAvatar(context.Background(), dto.TenantIdentity{Hostname: "alpha.test", Token: "token"}, body)
	var business *domain.Error
	var operation interface{ Operation() string }
	if !errors.As(err, &business) || business.Code != "unauthorized" || !errors.Is(err, objects.deleteErr) || !errors.As(err, &operation) || operation.Operation() != "delete uncommitted avatar" {
		t.Fatalf("清理失败丢失原错误或诊断：%v", err)
	}
}

func avatarSetup(t *testing.T) (Services, *avatarRepository, *avatarObjects, []byte) {
	t.Helper()
	repo := &avatarRepository{valid: true, user: domain.User{ID: "2", TenantID: "1", Role: "user", Status: "active"}}
	objects := &avatarObjects{values: make(map[string][]byte)}
	s := New(Dependencies{Persistence: avatarPersistence{repository: repo}, Objects: objects}, Config{})
	var body bytes.Buffer
	if err := png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return s, repo, objects, body.Bytes()
}

func TestAvatarUploadUsesObjectStorageAndPreservesPublicURL(t *testing.T) {
	s, _, objects, body := avatarSetup(t)
	ctx := context.Background()
	upload, err := s.Accounts.UploadAvatar(ctx, dto.TenantIdentity{Hostname: "alpha.test", Token: "token"}, body)
	if err != nil || !strings.HasPrefix(upload.AvatarURL, "/api/avatars/1/2/") || !strings.HasSuffix(upload.AvatarURL, ".png") {
		t.Fatalf("upload: %+v %v", upload, err)
	}
	name := strings.TrimPrefix(upload.AvatarURL, "/api/avatars/1/2/")
	object, err := s.Accounts.OpenAvatar(ctx, "1", "2", name)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	stored, err := io.ReadAll(object.Body)
	if err != nil || !bytes.Equal(stored, body) || objects.getKey != "avatars/1/2/"+name {
		t.Fatalf("opened wrong avatar: %q %v", objects.getKey, err)
	}
	for _, invalid := range []string{"../secret.png", "file.png", strings.Repeat("a", 32) + ".svg"} {
		if _, err := s.Accounts.OpenAvatar(ctx, "1", "2", invalid); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("unsafe name accepted: %q %v", invalid, err)
		}
	}
}

func TestAvatarUploadCleansOwnedObjectOnFailureOrCancellation(t *testing.T) {
	for _, failure := range []string{"partial put", "revoked session", "canceled request"} {
		t.Run(failure, func(t *testing.T) {
			s, repo, objects, body := avatarSetup(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch failure {
			case "partial put":
				objects.putErr = errors.New("object upload failed")
			case "revoked session":
				objects.afterPut = func() { repo.valid = false }
			case "canceled request":
				objects.afterPut = cancel
			}
			if _, err := s.Accounts.UploadAvatar(ctx, dto.TenantIdentity{Hostname: "alpha.test", Token: "token"}, body); err == nil {
				t.Fatal("failed upload succeeded")
			}
			if len(objects.values) != 0 {
				t.Fatalf("failed upload leaked objects: %v", objects.values)
			}
		})
	}
}

func TestProfileChecksObjectExistenceAndOwnership(t *testing.T) {
	s, repo, objects, _ := avatarSetup(t)
	nickname := "new nickname"
	name := strings.Repeat("a", 32) + ".png"
	for _, path := range []string{"/api/avatars/1/3/" + name, "/api/avatars/1/2/" + name} {
		if _, err := s.Accounts.Profile(context.Background(), dto.TenantIdentity{Hostname: "alpha.test", Token: "token"}, dto.ProfileInput{Nickname: &nickname, AvatarURL: &path}); err == nil {
			t.Fatal("missing or other account avatar accepted")
		}
		if repo.user.Nickname != "" || repo.user.AvatarURL != "" {
			t.Fatal("invalid profile changed stored account")
		}
	}
	path := "/api/avatars/1/2/" + name
	objects.values["avatars/1/2/"+name] = []byte("existing image")
	objects.statErr = errors.New("storage unavailable")
	_, err := s.Accounts.Profile(context.Background(), dto.TenantIdentity{Hostname: "alpha.test", Token: "token"}, dto.ProfileInput{Nickname: &nickname, AvatarURL: &path})
	if !errors.Is(err, objects.statErr) {
		t.Fatalf("infrastructure failure lost: %v", err)
	}
	objects.statErr = nil
	user, err := s.Accounts.Profile(context.Background(), dto.TenantIdentity{Hostname: "alpha.test", Token: "token"}, dto.ProfileInput{Nickname: &nickname, AvatarURL: &path})
	if err != nil || user.AvatarURL != path || repo.user.AvatarURL != path {
		t.Fatalf("owned persisted avatar rejected: %+v %v", user, err)
	}
}
