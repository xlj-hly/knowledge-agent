package domain

import "unicode/utf8"

// ValidateNickname 允许显式清空昵称，非空值最多 64 个 Unicode 字符。
func ValidateNickname(value string) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 64 {
		return Invalid("昵称最多 64 字符")
	}
	return nil
}

// ProfileChanges 校验完整资料保存契约，空字符串表示清空而非缺省。
func ProfileChanges(nickname, avatar *string) (UserChanges, error) {
	if nickname == nil || avatar == nil {
		return UserChanges{}, Invalid("昵称和头像字段均为必填")
	}
	if err := ValidateNickname(*nickname); err != nil {
		return UserChanges{}, err
	}
	if err := ValidateAvatar(*avatar); err != nil {
		return UserChanges{}, err
	}
	return UserChanges{Nickname: nickname, AvatarURL: avatar}, nil
}

// ValidateAccess 校验角色和状态的部分更新，不接受空操作。
func (c UserChanges) ValidateAccess() error {
	if c.Role == nil && c.Status == nil {
		return Invalid("没有可更新的字段")
	}
	if c.Role != nil && *c.Role != "admin" && *c.Role != "user" || c.Status != nil && *c.Status != "active" && *c.Status != "disabled" {
		return Invalid("角色或状态无效")
	}
	return nil
}

// LosesAdminAccess 表示本次变更会减少一名正常管理员，计数必须在企业排他锁内读取。
func (u User) LosesAdminAccess(c UserChanges) bool {
	return u.Role == "admin" && u.Status == "active" && (c.Role != nil && *c.Role != "admin" || c.Status != nil && *c.Status != "active")
}

// ChangeAccess 生成完整角色、状态变更，并保护企业最后一名正常管理员。
// activeAdmins 必须来自与本次写入同一企业锁保护的快照。
func (u User) ChangeAccess(c UserChanges, activeAdmins int64) (UserChanges, error) {
	if err := c.ValidateAccess(); err != nil {
		return UserChanges{}, err
	}
	if u.LosesAdminAccess(c) && activeAdmins <= 1 {
		return UserChanges{}, Conflict("至少保留一名正常企业管理员")
	}
	role, status := u.Role, u.Status
	if c.Role != nil {
		role = *c.Role
	}
	if c.Status != nil {
		status = *c.Status
	}
	return UserChanges{Role: &role, Status: &status}, nil
}

// Validated 校验企业部分更新并规范域名；部署保留域名由应用用例另行校验。
func (c CompanyChanges) Validated() (CompanyChanges, error) {
	if c.Name == nil && c.Hostname == nil && c.Status == nil && c.RegistrationEnabled == nil {
		return CompanyChanges{}, Invalid("没有可更新的字段")
	}
	if c.Name != nil {
		if err := ValidateName(*c.Name, 128, "企业名称"); err != nil {
			return CompanyChanges{}, err
		}
	}
	if c.Hostname != nil {
		host, err := NormalizeHostname(*c.Hostname)
		if err != nil {
			return CompanyChanges{}, err
		}
		c.Hostname = &host
	}
	if c.Status != nil && *c.Status != "active" && *c.Status != "disabled" {
		return CompanyChanges{}, Invalid("企业状态无效")
	}
	return c, nil
}

// RequiresSessionRevocation 标识企业停用或域名变更后必须撤销原有企业会话。
func (t Tenant) RequiresSessionRevocation(next Tenant) bool {
	return next.Status == "disabled" || next.Hostname != t.Hostname
}

// RequireActive 拒绝停用企业的业务操作。
func (t Tenant) RequireActive() error {
	if t.Status != "active" {
		return ErrDisabled
	}
	return nil
}

// RequireRegistration 在正常企业内实施密码自助注册开关。
func (t Tenant) RequireRegistration() error {
	if err := t.RequireActive(); err != nil {
		return err
	}
	if !t.RegistrationEnabled {
		return ErrRegistrationClosed
	}
	return nil
}

// RequireActive 拒绝停用账号的认证和业务操作。
func (u User) RequireActive() error {
	if u.Status != "active" {
		return ErrDisabled
	}
	return nil
}

// RequireAdmin 校验正常账号的企业管理员角色。
func (u User) RequireAdmin() error {
	if err := u.RequireActive(); err != nil {
		return err
	}
	if u.Role != "admin" {
		return Forbidden()
	}
	return nil
}
