package dto

// TenantIdentity 使用域名选择企业，并携带已解析的原始 Bearer 令牌。
// 域名仅用于选择企业；服务仍需校验令牌、账号与企业归属，禁止记录 Token。
type TenantIdentity struct{ Hostname, Token string }

// LoginInput 携带登录凭据；用户名在服务内规范化，密码不允许写入日志。
type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateUserInput 由企业管理员创建账号；未指定 Role 时默认为 user。
type CreateUserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

// ProfileInput 完整保存昵称和头像；两个字段都必填，nil 无效，空字符串表示清空。
type ProfileInput struct {
	Nickname  *string `json:"nickname"`
	AvatarURL *string `json:"avatar_url"`
}

// PasswordInput 携带当前密码和新密码；变更成功后撤销该身份的全部会话。
type PasswordInput struct {
	Current string `json:"current_password"`
	New     string `json:"new_password"`
}

// ResetPasswordInput 供企业管理员重置目标成员密码，不要求目标成员的旧密码。
type ResetPasswordInput struct {
	New string `json:"new_password"`
}

// UpdateUserInput 表示角色和状态的部分更新；nil 保留原值，最后管理员约束由服务校验。
type UpdateUserInput struct {
	Role   *string `json:"role"`
	Status *string `json:"status"`
}

// UpdateOwnTenantInput 仅允许企业管理员更新企业名称，nil 表示保留。
type UpdateOwnTenantInput struct {
	Name *string `json:"name"`
}

// CreateTenantInput 携带企业与首个管理员资料，两者必须在同一事务内创建。
type CreateTenantInput struct {
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
}

// UpdateTenantInput 供平台更新企业；nil 保留原值，停用或变更域名会撤销企业会话。
type UpdateTenantInput struct {
	Name     *string `json:"name"`
	Hostname *string `json:"hostname"`
	Status   *string `json:"status"`
}

// ResetAdminPasswordInput 按用户名定位现有企业管理员，不能借此新建或提升账号权限。
type ResetAdminPasswordInput struct {
	Username string `json:"username"`
	New      string `json:"new_password"`
}
