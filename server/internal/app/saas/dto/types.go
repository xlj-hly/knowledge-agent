// Package dto 定义 SaaS 用例的输入、输出及公开 JSON 契约。
package dto

import (
	"time"
)

// Tenant 表示企业及其生命周期状态。
type Tenant struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Hostname            string    `json:"hostname"`
	Status              string    `json:"status"`
	RegistrationEnabled bool      `json:"registration_enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// User 表示归属单一企业的账号及其资料、角色和状态。
type User struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlatformAdmin 表示与企业账号隔离的平台身份，不能作为企业用户使用。
type PlatformAdmin struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}
