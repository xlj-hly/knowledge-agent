package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"go-server/internal/app/saas/dto"
	"go-server/internal/app/storage"
	"go-server/internal/domain/saas"
)

var avatarFilename = regexp.MustCompile(`^[0-9a-f]{32}\.(png|jpg)$`)
var avatarIdentity = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

type avatarCleanupFailure struct{ cause error }

func (e *avatarCleanupFailure) Error() string     { return "清理未提交头像失败" }
func (e *avatarCleanupFailure) Unwrap() error     { return e.cause }
func (e *avatarCleanupFailure) Operation() string { return "delete uncommitted avatar" }

func avatarObjectKey(tenant, user, name string) (string, error) {
	if !avatarIdentity.MatchString(tenant) || !avatarIdentity.MatchString(user) || !avatarFilename.MatchString(name) {
		return "", storage.ErrNotFound
	}
	if _, err := domain.ParseID(tenant); err != nil {
		return "", storage.ErrNotFound
	}
	if _, err := domain.ParseID(user); err != nil {
		return "", storage.ErrNotFound
	}
	return "avatars/" + tenant + "/" + user + "/" + name, nil
}

func (s *runtime) ownsAvatar(ctx context.Context, tenant, user, url string) (bool, error) {
	parts := strings.Split(url, "/")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "api" || parts[2] != "avatars" || parts[3] != tenant || parts[4] != user {
		return false, nil
	}
	key, err := avatarObjectKey(tenant, user, parts[5])
	if err != nil {
		return false, nil
	}
	_, err = s.objects.Stat(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// UploadAvatar 在数据库事务外验证并上传图片，随后在身份锁内复核会话和归属。
// URL 仅在复核成功后交付；资料保存仍通过 Profile 完成。
func (s *Accounts) UploadAvatar(ctx context.Context, id dto.TenantIdentity, body []byte) (out dto.AvatarUpload, err error) {
	format, err := domain.ValidateAvatarImage(body)
	if err != nil {
		return out, err
	}
	account, err := s.me(ctx, id)
	if err != nil {
		return out, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return out, err
	}
	ext, contentType := ".png", "image/png"
	if format == "jpeg" {
		ext, contentType = ".jpg", "image/jpeg"
	}
	name := hex.EncodeToString(random[:]) + ext
	key, err := avatarObjectKey(account.Tenant.ID, account.User.ID, name)
	if err != nil {
		return out, err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		// Put 失败也可能留下部分对象；清理只触及本次随机键。
		// 请求取消不能取消资源回收，独立的短超时限制后台清理时间。
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if removeErr := s.objects.Delete(cleanup, key); removeErr != nil && !errors.Is(removeErr, storage.ErrNotFound) {
			err = errors.Join(err, &avatarCleanupFailure{cause: removeErr})
		}
	}()
	if err := s.objects.Put(ctx, key, bytes.NewReader(body), int64(len(body)), contentType); err != nil {
		return out, err
	}
	err = s.tenant(ctx, id, domain.Shared, domain.Shared, true, false, func(_ domain.Store, state tenantState) error {
		if state.tenant.ID != account.Tenant.ID || state.account.User.ID != account.User.ID {
			return domain.Unauthorized()
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	committed = true
	return dto.AvatarUpload{AvatarURL: "/api/" + key}, nil
}

// OpenAvatar 仅打开受控随机图片键，返回对象的 Body 由调用方关闭。
func (s *Accounts) OpenAvatar(ctx context.Context, tenant, user, name string) (storage.Object, error) {
	key, err := avatarObjectKey(tenant, user, name)
	if err != nil {
		return storage.Object{}, err
	}
	return s.objects.Get(ctx, key)
}
