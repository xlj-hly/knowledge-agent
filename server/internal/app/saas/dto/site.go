package dto

// Site 为页面入口提供站点类型；未知域名不回退到平台或默认租户。
type Site struct {
	Kind     string  `json:"kind"`
	Hostname string  `json:"hostname"`
	Tenant   *Tenant `json:"tenant,omitempty"`
}
