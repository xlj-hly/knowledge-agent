// Package repository 通过私有 GORM 模型访问 SaaS 数据，不拥有业务事务边界。
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go-server/internal/app/saas/port"
	"go-server/internal/domain/saas"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store 将操作绑定到显式连接或事务；身份、业务校验和事务范围由应用用例决定。
// 除专用维护操作外，企业账号和企业会话查询必须显式携带企业范围。
type Store struct{ db *gorm.DB }

var _ port.Persistence = (*Store)(nil)
var _ domain.Store = (*Store)(nil)

type operationError struct {
	operation string
	cause     error
}

func (e *operationError) Error() string {
	return fmt.Sprintf("repository %s: %v", e.operation, e.cause)
}
func (e *operationError) Unwrap() error     { return e.cause }
func (e *operationError) Operation() string { return e.operation }

// New 绑定调用方提供的 GORM 连接或事务，不接管连接池清理责任。
func New(db *gorm.DB) *Store { return &Store{db: db} }

// Ping 使用调用方上下文检查底层数据库连接，不改变数据。
func (s *Store) Ping(ctx context.Context) error {
	pool, err := s.db.DB()
	if err != nil {
		return checked("obtain SQL pool", err)
	}
	return checked("ping database", pool.PingContext(ctx))
}

// WithContext 返回使用指定上下文的连接视图，不创建事务，也不修改原 Store。
func (s *Store) WithContext(ctx context.Context) domain.Store { return New(s.db.WithContext(ctx)) }

// Transaction 执行回调并负责提交或回滚；回调中的 Store 不得在事务结束后继续使用。
func (s *Store) Transaction(ctx context.Context, fn func(domain.Store) error) error {
	return checked("transaction", s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(New(tx)) }))
}

func locked(q *gorm.DB, lock domain.Lock) *gorm.DB {
	switch lock {
	case domain.Unlocked:
		return q
	case domain.Shared:
		return q.Clauses(clause.Locking{Strength: "SHARE"})
	case domain.Exclusive:
		return q.Clauses(clause.Locking{Strength: "UPDATE"})
	default:
		// 未知模式不能悄悄降为无锁；查询终止时由正常错误路径返回。
		failed := q.Session(&gorm.Session{})
		_ = failed.AddError(fmt.Errorf("invalid persistence lock mode"))
		return failed
	}
}
func checked(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NotFound()
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return domain.Conflict("域名或用户名已存在")
	}
	var business *domain.Error
	if errors.As(err, &business) {
		return err
	}
	var existing *operationError
	if errors.As(err, &existing) {
		return err
	}
	return &operationError{operation: operation, cause: err}
}

// TenantByHost 按已规范化域名读取企业，lock 由服务对应具体不变量选择。
func (s *Store) TenantByHost(host string, lock domain.Lock) (domain.Tenant, error) {
	var m tenantModel
	err := locked(s.db, lock).Where("hostname = ?", host).Take(&m).Error
	return m.view(), checked("find tenant by hostname", err)
}

// TenantByID 按 ID 读取企业，允许平台管理路径查询停用企业。
func (s *Store) TenantByID(id string, lock domain.Lock) (domain.Tenant, error) {
	var m tenantModel
	err := locked(s.db, lock).Where("id = ?", id).Take(&m).Error
	return m.view(), checked("find tenant by ID", err)
}

// CreateTenant 创建正常企业；调用方需在同一事务创建首个管理员。
func (s *Store) CreateTenant(name, host string) (domain.Tenant, error) {
	m := tenantModel{Name: name, Hostname: host, Status: "active"}
	err := s.db.Create(&m).Error
	return m.view(), checked("create tenant", err)
}

// UpdateTenant 写入服务构造的企业字段并更新修改时间；调用方必须先持有企业排他锁。
func (s *Store) UpdateTenant(id string, changes domain.CompanyChanges) (domain.Tenant, error) {
	values := map[string]any{"updated_at": time.Now().UTC()}
	if changes.Name != nil {
		values["name"] = *changes.Name
	}
	if changes.Hostname != nil {
		values["hostname"] = *changes.Hostname
	}
	if changes.Status != nil {
		values["status"] = *changes.Status
	}
	if err := s.db.Model(&tenantModel{}).Where("id = ?", id).Updates(values).Error; err != nil {
		return domain.Tenant{}, checked("update tenant", err)
	}
	return s.TenantByID(id, domain.Unlocked)
}

// UserByID 同时匹配企业与账号 ID，避免跨企业查询；按 lock 加账号行锁。
func (s *Store) UserByID(tenantID, id string, lock domain.Lock) (domain.Account, error) {
	var m userModel
	err := locked(s.db, lock).Where("tenant_id = ? AND id = ?", tenantID, id).Take(&m).Error
	return domain.Account{User: m.view(), PasswordHash: m.PasswordHash}, checked("find tenant user", err)
}

// UserByName 在单一企业范围内查找凭据，用户名需由服务先规范化。
func (s *Store) UserByName(tenantID, name string, lock domain.Lock) (domain.Account, error) {
	var m userModel
	err := locked(s.db, lock).Where("tenant_id = ? AND username = ?", tenantID, name).Take(&m).Error
	return domain.Account{User: m.view(), PasswordHash: m.PasswordHash}, checked("find tenant credentials", err)
}

// CreateUser 使用已验证的角色和密码散列创建账号，归属企业需由服务先确认存在。
func (s *Store) CreateUser(tenantID, username, hash, nickname, role string) (domain.User, error) {
	id, err := domain.ParseID(tenantID)
	if err != nil {
		return domain.User{}, err
	}
	m := userModel{TenantID: id, Username: username, PasswordHash: hash, Nickname: nickname, Role: role, Status: "active"}
	err = s.db.Create(&m).Error
	return m.view(), checked("create tenant user", err)
}

// UpdateUser 只更新指定企业内的账号；changes 必须来自服务允许字段，不能直接使用请求数据。
func (s *Store) UpdateUser(tenantID, id string, changes domain.UserChanges) (domain.User, error) {
	values := map[string]any{"updated_at": time.Now().UTC()}
	if changes.Nickname != nil {
		values["nickname"] = *changes.Nickname
	}
	if changes.AvatarURL != nil {
		values["avatar_url"] = *changes.AvatarURL
	}
	if changes.Role != nil {
		values["role"] = *changes.Role
	}
	if changes.Status != nil {
		values["status"] = *changes.Status
	}
	if changes.PasswordHash != nil {
		values["password_hash"] = *changes.PasswordHash
	}
	if err := s.db.Model(&userModel{}).Where("tenant_id = ? AND id = ?", tenantID, id).Updates(values).Error; err != nil {
		return domain.User{}, checked("update tenant user", err)
	}
	a, err := s.UserByID(tenantID, id, domain.Unlocked)
	return a.User, err
}

// ActiveAdminCount 统计企业有效管理员；保护最后管理员时需先持有企业排他锁。
func (s *Store) ActiveAdminCount(tenantID string) (int64, error) {
	var n int64
	err := s.db.Model(&userModel{}).Where("tenant_id = ? AND role = ? AND status = ?", tenantID, "admin", "active").Count(&n).Error
	return n, checked("count active tenant administrators", err)
}
func search(q string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
}

// ListUsers 在指定企业内分页搜索账号，不返回密码散列；分页条件需已验证。
func (s *Store) ListUsers(tenantID string, p domain.Pagination) (domain.Page[domain.User], error) {
	r := domain.Page[domain.User]{Items: []domain.User{}, Page: p.Page, Size: p.Size}
	pattern := search(p.Query)
	q := s.db.Model(&userModel{}).Where("tenant_id = ? AND (username ILIKE ? OR nickname ILIKE ?)", tenantID, pattern, pattern)
	if err := q.Count(&r.Total).Error; err != nil {
		return r, checked("count tenant users", err)
	}
	var models []userModel
	if err := q.Order("created_at DESC, id DESC").Limit(p.Size).Offset((p.Page - 1) * p.Size).Find(&models).Error; err != nil {
		return r, checked("list tenant users", err)
	}
	for _, m := range models {
		r.Items = append(r.Items, m.view())
	}
	return r, nil
}

// ListTenants 分页搜索平台企业列表；平台权限和分页条件由服务先校验。
func (s *Store) ListTenants(p domain.Pagination) (domain.Page[domain.Tenant], error) {
	r := domain.Page[domain.Tenant]{Items: []domain.Tenant{}, Page: p.Page, Size: p.Size}
	pattern := search(p.Query)
	q := s.db.Model(&tenantModel{}).Where("name ILIKE ? OR hostname ILIKE ?", pattern, pattern)
	if err := q.Count(&r.Total).Error; err != nil {
		return r, checked("count tenants", err)
	}
	var models []tenantModel
	if err := q.Order("created_at DESC, id DESC").Limit(p.Size).Offset((p.Page - 1) * p.Size).Find(&models).Error; err != nil {
		return r, checked("list tenants", err)
	}
	for _, m := range models {
		r.Items = append(r.Items, m.view())
	}
	return r, nil
}

// AdminByID 按平台身份 ID 查询凭据，不能用企业账号 ID 替代。
func (s *Store) AdminByID(id string, lock domain.Lock) (domain.PlatformAccount, error) {
	var m adminModel
	err := locked(s.db, lock).Where("id = ?", id).Take(&m).Error
	return domain.PlatformAccount{Admin: m.view(), PasswordHash: m.PasswordHash}, checked("find platform administrator", err)
}

// AdminByName 按规范化平台用户名读取凭据，lock 与平台改密不变量对应。
func (s *Store) AdminByName(name string, lock domain.Lock) (domain.PlatformAccount, error) {
	var m adminModel
	err := locked(s.db, lock).Where("username = ?", name).Take(&m).Error
	return domain.PlatformAccount{Admin: m.view(), PasswordHash: m.PasswordHash}, checked("find platform credentials", err)
}

// UpdateAdminPassword 写入已生成的散列；调用方需先持有平台账号排他锁。
func (s *Store) UpdateAdminPassword(id, hash string) error {
	return checked("update platform password", s.db.Model(&adminModel{}).Where("id = ?", id).Updates(map[string]any{"password_hash": hash, "updated_at": time.Now().UTC()}).Error)
}

// BootstrapLock 在当前事务内串行化首次平台初始化，解决尚无身份行可加锁的情况。
func (s *Store) BootstrapLock() error {
	return checked("lock platform bootstrap", s.db.Exec("SELECT pg_advisory_xact_lock(?)", int64(723451992)).Error)
}

// AdminCount 返回平台身份总数；初始化前检查需与 BootstrapLock 处于同一事务。
func (s *Store) AdminCount() (int64, error) {
	var n int64
	err := s.db.Model(&adminModel{}).Count(&n).Error
	return n, checked("count platform administrators", err)
}

// CreateAdmin 使用已验证用户名和密码散列创建平台账号，不负责首次创建权限判断。
func (s *Store) CreateAdmin(name, hash string) error {
	return checked("create platform administrator", s.db.Create(&adminModel{Username: name, PasswordHash: hash}).Error)
}

// SessionSubject 按身份范围返回仍有效的会话主体；企业会话必须同时匹配 tenantID。
// 使用 clock_timestamp()，避免事务开始时间固定导致锁等待延长会话有效期。
func (s *Store) SessionSubject(hash, scope, tenantID string) (string, error) {
	q := s.db.Where("token_hash = ? AND scope = ? AND expires_at > clock_timestamp()", hash, scope)
	if scope == "tenant" {
		q = q.Where("tenant_id = ?", tenantID)
	} else {
		q = q.Where("tenant_id IS NULL")
	}
	var m sessionModel
	err := q.Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", domain.Unauthorized()
	}
	if err != nil {
		return "", checked("find active session", err)
	}
	return fmt.Sprint(m.SubjectID), nil
}

// CreateSession 仅保存令牌摘要；tenant 范围必须提供企业 ID，platform 范围使用空企业 ID。
func (s *Store) CreateSession(hash, scope, subject, tenantID string, expires time.Time) error {
	id, err := domain.ParseID(subject)
	if err != nil {
		return err
	}
	m := sessionModel{TokenHash: hash, Scope: scope, SubjectID: id, ExpiresAt: expires}
	if tenantID != "" {
		tenant, err := domain.ParseID(tenantID)
		if err != nil {
			return err
		}
		m.TenantID = &tenant
	}
	return checked("create session", s.db.Create(&m).Error)
}

// RevokeToken 按摘要撤销单个会话，重复撤销不产生额外副作用。
func (s *Store) RevokeToken(hash string) error {
	return checked("revoke session", s.db.Where("token_hash = ?", hash).Delete(&sessionModel{}).Error)
}

// RevokeUser 仅撤销指定企业与账号的会话，不影响平台或其他企业身份。
func (s *Store) RevokeUser(tenantID, id string) error {
	return checked("revoke tenant user sessions", s.db.Where("scope = ? AND tenant_id = ? AND subject_id = ?", "tenant", tenantID, id).Delete(&sessionModel{}).Error)
}

// RevokeTenant 撤销企业全部会话，用于企业停用或域名切换。
func (s *Store) RevokeTenant(id string) error {
	return checked("revoke tenant sessions", s.db.Where("scope = ? AND tenant_id = ?", "tenant", id).Delete(&sessionModel{}).Error)
}

// RevokeAdmin 仅撤销指定平台账号的会话，不影响企业账号。
func (s *Store) RevokeAdmin(id string) error {
	return checked("revoke platform sessions", s.db.Where("scope = ? AND subject_id = ?", "platform", id).Delete(&sessionModel{}).Error)
}

// CleanupSessions 删除按数据库当前时钟已到期的会话，由调用方控制维护上下文和调度。
func (s *Store) CleanupSessions(ctx context.Context) error {
	return checked("clean expired sessions", s.db.WithContext(ctx).Where("expires_at <= clock_timestamp()").Delete(&sessionModel{}).Error)
}
