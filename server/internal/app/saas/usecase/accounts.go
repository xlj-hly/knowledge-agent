package usecase

import (
	"context"
	"strings"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
	"golang.org/x/crypto/bcrypt"
)

// Profile 完整保存当前账号的昵称和头像；两个字段都必填，显式空字符串清空。
func (s *Accounts) Profile(ctx context.Context, id dto.TenantIdentity, in dto.ProfileInput) (out dto.User, err error) {
	changes, err := domain.ProfileChanges(in.Nickname, in.AvatarURL)
	if err != nil {
		return out, err
	}
	var checkedTenant, checkedUser string
	if strings.HasPrefix(*in.AvatarURL, "/") {
		account, err := s.me(ctx, id)
		if err != nil {
			return out, err
		}
		checkedTenant, checkedUser = account.Tenant.ID, account.User.ID
		// 对象使用不可变随机键，当前没有用户删除入口；远程存在性校验不持有数据库锁。
		// 完成后仍在写事务内重新验证会话及所属身份，外部等待不能延长认证有效期。
		owned, err := s.ownsAvatar(ctx, checkedTenant, checkedUser, *in.AvatarURL)
		if err != nil {
			return out, err
		}
		if !owned {
			return out, domain.Invalid("头像文件不存在或不属于当前账号")
		}
	}
	err = s.tenant(ctx, id, domain.Shared, domain.Exclusive, true, false, func(tx domain.Store, state tenantState) error {
		if checkedTenant != "" && (state.tenant.ID != checkedTenant || state.account.User.ID != checkedUser) {
			return domain.Unauthorized()
		}
		user, err := tx.UpdateUser(state.tenant.ID, state.account.User.ID, changes)
		out = userDTO(user)
		return err
	})
	return
}

// ChangePassword 校验当前密码、设置新密码并撤销该账号的全部会话。
// 密码校验和散列在写事务前完成，取得账号排他锁后再次核对凭据快照。
func (s *Accounts) ChangePassword(ctx context.Context, id dto.TenantIdentity, in dto.PasswordInput) (dto.OK, error) {
	if in.Current == in.New {
		return dto.OK{}, domain.Invalid("新密码不能与当前密码相同")
	}
	hash, err := passwordHash(in.New)
	if err != nil {
		return dto.OK{}, err
	}
	store := s.store.WithContext(ctx)
	host, err := domain.NormalizeHostname(id.Hostname)
	if err != nil {
		return dto.OK{}, err
	}
	tenant, err := store.TenantByHost(host, domain.Unlocked)
	if err != nil {
		return dto.OK{}, err
	}
	userID, err := store.SessionSubject(tokenDigest(id.Token), "tenant", tenant.ID)
	if err != nil {
		return dto.OK{}, err
	}
	snapshot, err := store.UserByID(tenant.ID, userID, domain.Unlocked)
	if err != nil {
		return dto.OK{}, missingIdentity(err)
	}
	if snapshot.PasswordHash == "" {
		return dto.OK{}, domain.Invalid("账号尚未设置密码，请联系企业管理员")
	}
	if bcrypt.CompareHashAndPassword([]byte(snapshot.PasswordHash), []byte(in.Current)) != nil {
		return dto.OK{}, &domain.Error{Code: "invalid_password", Message: "当前密码错误"}
	}
	err = s.tenant(ctx, id, domain.Shared, domain.Exclusive, true, false, func(tx domain.Store, state tenantState) error {
		if state.account.PasswordHash != snapshot.PasswordHash {
			return domain.Unauthorized()
		}
		if _, err := tx.UpdateUser(state.tenant.ID, state.account.User.ID, domain.UserChanges{PasswordHash: &hash}); err != nil {
			return err
		}
		return tx.RevokeUser(state.tenant.ID, state.account.User.ID)
	})
	return dto.OK{OK: true}, err
}

// PlatformPassword 校验并变更当前平台密码，成功后撤销该平台账号的全部会话。
func (s *Accounts) PlatformPassword(ctx context.Context, token string, in dto.PasswordInput) (dto.OK, error) {
	if in.Current == in.New {
		return dto.OK{}, domain.Invalid("新密码不能与当前密码相同")
	}
	hash, err := passwordHash(in.New)
	if err != nil {
		return dto.OK{}, err
	}
	store := s.store.WithContext(ctx)
	id, err := store.SessionSubject(tokenDigest(token), "platform", "")
	if err != nil {
		return dto.OK{}, err
	}
	snapshot, err := store.AdminByID(id, domain.Unlocked)
	if err != nil {
		return dto.OK{}, missingIdentity(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(snapshot.PasswordHash), []byte(in.Current)) != nil {
		return dto.OK{}, &domain.Error{Code: "invalid_password", Message: "当前密码错误"}
	}
	err = s.platform(ctx, token, domain.Exclusive, func(tx domain.Store, state platformState) error {
		if state.account.PasswordHash != snapshot.PasswordHash {
			return domain.Unauthorized()
		}
		if err := tx.UpdateAdminPassword(state.account.Admin.ID, hash); err != nil {
			return err
		}
		return tx.RevokeAdmin(state.account.Admin.ID)
	})
	return dto.OK{OK: true}, err
}

// ListUsers 要求当前企业管理员身份，仅搜索该企业内的账号。
func (s *Accounts) ListUsers(ctx context.Context, id dto.TenantIdentity, p dto.Pagination) (out dto.Page[dto.User], err error) {
	if err := p.Validate(); err != nil {
		return out, err
	}
	err = s.tenant(ctx, id, domain.Shared, domain.Shared, true, true, func(tx domain.Store, state tenantState) error {
		page, err := tx.ListUsers(state.tenant.ID, p.Domain())
		out = usersDTO(page)
		return err
	})
	return
}

// CreateUser 要求企业管理员身份，先完成纯校验和密码散列，再在事务内创建账号。
func (s *Accounts) CreateUser(ctx context.Context, id dto.TenantIdentity, in dto.CreateUserInput) (out dto.User, err error) {
	if in.Role == "" {
		in.Role = "user"
	}
	name, hash, err := prepareUser(in.Username, in.Password, in.Nickname, in.Role)
	if err != nil {
		return out, err
	}
	err = s.tenant(ctx, id, domain.Shared, domain.Shared, true, true, func(tx domain.Store, state tenantState) error {
		user, err := tx.CreateUser(state.tenant.ID, name, hash, in.Nickname, in.Role)
		out = userDTO(user)
		return err
	})
	return
}

// UpdateUser 修改本企业账号角色或状态；保护最后有效管理员并撤销发生变化账号的会话。
func (s *Accounts) UpdateUser(ctx context.Context, id dto.TenantIdentity, userID string, in dto.UpdateUserInput) (out dto.User, err error) {
	if _, err := domain.ParseID(userID); err != nil {
		return out, err
	}
	changes := domain.UserChanges{Role: in.Role, Status: in.Status}
	if err := changes.ValidateAccess(); err != nil {
		return out, err
	}
	// 企业排他锁串行化有效管理员变更，防止两个管理员相互降权后无人可管理。
	err = s.tenant(ctx, id, domain.Exclusive, domain.Shared, true, true, func(tx domain.Store, state tenantState) error {
		account, err := tx.UserByID(state.tenant.ID, userID, domain.Unlocked)
		if err != nil {
			return err
		}
		u := account.User
		var count int64
		if u.LosesAdminAccess(changes) {
			count, err = tx.ActiveAdminCount(state.tenant.ID)
			if err != nil {
				return err
			}
		}
		changes, err := u.ChangeAccess(changes, count)
		if err != nil {
			return err
		}
		user, err := tx.UpdateUser(state.tenant.ID, userID, changes)
		if err != nil {
			return err
		}
		out = userDTO(user)
		if *changes.Role != u.Role || *changes.Status != u.Status {
			return tx.RevokeUser(state.tenant.ID, userID)
		}
		return nil
	})
	return
}

// ResetPassword 要求本企业管理员身份，重置本企业目标账号并撤销其全部会话。
func (s *Accounts) ResetPassword(ctx context.Context, id dto.TenantIdentity, userID string, in dto.ResetPasswordInput) (dto.OK, error) {
	if _, err := domain.ParseID(userID); err != nil {
		return dto.OK{}, err
	}
	hash, err := passwordHash(in.New)
	if err != nil {
		return dto.OK{}, err
	}
	// 跨账号重置先取得企业排他锁，再访问双方身份，防止两个管理员相互重置形成锁环。
	err = s.tenant(ctx, id, domain.Exclusive, domain.Shared, true, true, func(tx domain.Store, state tenantState) error {
		if _, err := tx.UserByID(state.tenant.ID, userID, domain.Unlocked); err != nil {
			return err
		}
		if _, err := tx.UpdateUser(state.tenant.ID, userID, domain.UserChanges{PasswordHash: &hash}); err != nil {
			return err
		}
		return tx.RevokeUser(state.tenant.ID, userID)
	})
	return dto.OK{OK: true}, err
}
