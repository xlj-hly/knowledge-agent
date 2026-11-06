// Package domain 定义 SaaS 领域实体、纯规则及错误类别。
package domain

import (
	"time"
)

// Tenant 表示企业及其生命周期状态。
type Tenant struct {
	ID                  string
	Name                string
	Hostname            string
	Status              string
	RegistrationEnabled bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// User 表示归属单一企业的账号及其资料、角色和状态。
type User struct {
	ID        string
	TenantID  string
	Username  string
	Nickname  string
	AvatarURL string
	Role      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PlatformAdmin 表示与企业账号隔离的平台身份，不能作为企业用户使用。
type PlatformAdmin struct {
	ID        string
	Username  string
	CreatedAt time.Time
}
