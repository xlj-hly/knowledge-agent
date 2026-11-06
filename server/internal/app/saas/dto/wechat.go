package dto

// WeChatLoginInput 只接收微信平台签发的临时 code，客户端不得指定身份或角色。
type WeChatLoginInput struct {
	Code string `json:"code"`
}

// WeChatPasswordInput 使用新code重新验证微信身份，不要求用户知道随机初始密码。
type WeChatPasswordInput struct {
	Code string `json:"code"`
	New  string `json:"new_password"`
}
