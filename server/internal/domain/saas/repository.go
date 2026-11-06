// Package domain 定义 SaaS 领域实体、规则与仓储契约。
package domain

import "time"

// Store 聚合同一连接或事务内的存取能力，由业务用例决定锁模式和操作顺序。
type Store interface {
	CompanyStore
	UserStore
	AdminStore
	SessionStore
	WeChatStore
}

// Lock 指定业务不变量所需的锁模式，锁保持至所属事务结束。
type Lock uint8

const (
	// Unlocked 用于锁外快照和已由其他锁保护的读取。
	Unlocked Lock = iota
	// Shared 允许并发读取，同时与企业或账号状态变更互斥。
	Shared
	// Exclusive 串行化同一身份写入或企业范围的生命周期变更。
	Exclusive
)

// CompanyStore 访问企业及其生命周期状态，不隐式选择默认企业。
type CompanyStore interface {
	TenantByHost(string, Lock) (Tenant, error)
	TenantByID(string, Lock) (Tenant, error)
	CreateTenant(string, string) (Tenant, error)
	UpdateTenant(string, CompanyChanges) (Tenant, error)
	ListTenants(Pagination) (Page[Tenant], error)
}

// Account 是包含密码散列的内部凭据快照，禁止进入公开响应或日志。
type Account struct {
	User         User
	PasswordHash string
}

// UserStore 的所有账号操作均显式限定企业范围。
type UserStore interface {
	UserByID(string, string, Lock) (Account, error)
	UserByName(string, string, Lock) (Account, error)
	CreateUser(string, string, string, string, string) (User, error)
	UpdateUser(string, string, UserChanges) (User, error)
	ActiveAdminCount(string) (int64, error)
	ListUsers(string, Pagination) (Page[User], error)
}

// CompanyChanges 是企业资料的受控部分更新字段；nil 表示保留原值。
type CompanyChanges struct {
	Name, Hostname, Status *string
}

// UserChanges 是账号资料和状态的受控部分更新字段；nil 表示保留原值。
type UserChanges struct {
	Nickname, AvatarURL, Role, Status *string
	PasswordHash                      *string
}

// PlatformAccount 是独立平台身份的内部凭据快照，密码散列不得公开。
type PlatformAccount struct {
	Admin        PlatformAdmin
	PasswordHash string
}

// AdminStore 访问总后台账号；初始化锁保护尚无账号行时的首次创建。
type AdminStore interface {
	AdminByID(string, Lock) (PlatformAccount, error)
	AdminByName(string, Lock) (PlatformAccount, error)
	UpdateAdminPassword(string, string) error
	BootstrapLock() error
	AdminCount() (int64, error)
	CreateAdmin(string, string) error
}

// SessionStore 只保存令牌摘要，企业会话始终携带企业范围。
type SessionStore interface {
	SessionSubject(string, string, string) (string, error)
	CreateSession(string, string, string, string, time.Time) error
	RevokeToken(string) error
	RevokeUser(string, string) error
	RevokeTenant(string) error
	RevokeAdmin(string) error
}

// WeChatConfiguration 是服务端配置快照，禁止直接序列化到响应或日志。
type WeChatConfiguration struct {
	TenantID, AppID, AppSecret string
	Version                    int64
}

// WeChatStore 在企业锁内维护配置和已验证身份，首次建号需与会话一起提交。
type WeChatStore interface {
	WeChatConfig(string) (WeChatConfiguration, error)
	SaveWeChatConfig(WeChatConfiguration) error
	LockWeChatIdentity(string, string, string) error
	WeChatUserID(string, string, string) (string, error)
	CreateWeChatUser(string, string, string, string, string) (User, error)
}
