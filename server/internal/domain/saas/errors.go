package domain

// Error 使用稳定 Code 表达业务失败，Message 为可公开的提示，不依赖 HTTP 状态码。
type Error struct{ Code, Message string }

// Error 返回稳定错误类别，公开提示由 Message 单独提供。
func (e *Error) Error() string { return e.Code }

// Invalid 构造可公开的输入错误；message 不得包含底层数据库信息或敏感值。
func Invalid(message string) error { return &Error{"invalid_input", message} }

// Unauthorized 表示会话缺失或确认失效，不用于数据库和网络故障。
func Unauthorized() error { return &Error{"unauthorized", "登录已失效，请重新登录"} }

// Forbidden 表示已识别身份无权执行操作，不代表凭据失效。
func Forbidden() error { return &Error{"forbidden", "无权执行此操作"} }

// NotFound 表示当前查询范围内没有目标记录，不暴露其他企业记录是否存在。
func NotFound() error { return &Error{"not_found", "记录不存在"} }

// Conflict 表示业务状态冲突；message 必须适合直接展示给客户端。
func Conflict(message string) error { return &Error{"conflict", message} }

// ErrCredentials 表示已确认的用户名或密码错误，与基础设施故障区分。
var ErrCredentials = &Error{"invalid_credentials", "用户名或密码错误"}

// ErrDisabled 表示企业或账号已停用。
var ErrDisabled = &Error{"disabled", "企业或账号已停用"}
