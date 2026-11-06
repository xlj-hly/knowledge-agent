package dto

import (
	"time"
)

// TenantSession 返回企业用户及企业；仅签发时包含原始 Token 与 ExpiresAt。
type TenantSession struct {
	Token     string     `json:"token,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	User      User       `json:"user"`
	Tenant    Tenant     `json:"tenant"`
}

// PlatformSession 返回独立平台身份；仅签发时包含原始 Token 与 ExpiresAt。
type PlatformSession struct {
	Token     string        `json:"token,omitempty"`
	ExpiresAt *time.Time    `json:"expires_at,omitempty"`
	Admin     PlatformAdmin `json:"admin"`
}

// CreatedTenant 返回原子创建的企业及首个管理员，账号不包含凭据。
type CreatedTenant struct {
	Tenant Tenant `json:"tenant"`
	User   User   `json:"user"`
}

// OK 表示已完成的无数据操作；调用方仍必须检查伴随返回的 error。
type OK struct {
	OK bool `json:"ok"`
}
