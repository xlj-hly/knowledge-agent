// Package wechat 封装微信小程序 code2session 协议，不拥有企业、用户或本系统会话。
package wechat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	code2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"
	maxResponseSize = 16 * 1024
)

// Credentials 是仅限服务端使用的小程序凭据，不得进入客户端响应或日志。
type Credentials struct{ AppID, AppSecret string }

// Identity 只包含已由微信验证的身份；会话密钥不离开协议适配层。
type Identity struct{ AppID, OpenID string }

// Error 提供稳定故障分类，不保存上游响应、请求 URL、临时 code 或小程序秘密。
// Code 为 invalid_code、blocked、rate_limited、unavailable 或 upstream_error。
type Error struct {
	Code  string
	cause error
}

func (e *Error) Error() string {
	switch e.Code {
	case "invalid_code":
		return "微信登录凭据无效，请重新登录"
	case "blocked":
		return "当前微信账号无法完成登录"
	case "rate_limited":
		return "微信登录尝试过于频繁，请稍后重试"
	case "unavailable":
		return "微信登录暂时不可用，请稍后重试"
	default:
		return "微信登录服务响应异常，请稍后重试"
	}
}

// Unwrap 仅暴露标准上下文取消原因，绝不保留可能携带敏感 URL 的网络错误。
func (e *Error) Unwrap() error { return e.cause }

// Client 复用 HTTP 连接，每次交换均遵守调用方取消和独立等待期限。
type Client struct {
	httpClient *http.Client
	endpoint   string
	timeout    time.Duration
}

// New 创建访问官方 code2session 的客户端；不自动重试可能已被消费的临时 code。
func New() *Client {
	return &Client{
		httpClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		endpoint:   code2SessionURL,
		timeout:    5 * time.Second,
	}
}

func printableASCII(value string, limit int) bool {
	if len(value) == 0 || len(value) > limit {
		return false
	}
	for i := range len(value) {
		if value[i] < 33 || value[i] > 126 {
			return false
		}
	}
	return true
}

func validIdentity(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i := range len(value) {
		c := value[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func failure(code string) error { return &Error{Code: code} }

// Exchange 用一次性 code 验证微信身份；调用方负责在数据库事务和业务锁外执行。
// 返回的 AppID 来自调用方选定的服务端配置，不接受客户端指定身份或小程序凭据。
func (c *Client) Exchange(ctx context.Context, credentials Credentials, code string) (Identity, error) {
	if !printableASCII(code, 512) {
		return Identity{}, failure("invalid_code")
	}
	if c == nil || c.httpClient == nil || c.timeout <= 0 || !printableASCII(credentials.AppID, 128) || !printableASCII(credentials.AppSecret, 256) {
		return Identity{}, failure("unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	query := url.Values{"appid": {credentials.AppID}, "secret": {credentials.AppSecret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return Identity{}, failure("unavailable")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		// net/http 的错误会附带完整 URL，不能作为错误链、日志或用户提示透传。
		return Identity{}, &Error{Code: "unavailable", cause: ctx.Err()}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return Identity{}, failure("rate_limited")
	}
	if response.StatusCode >= 500 {
		return Identity{}, failure("unavailable")
	}
	if response.StatusCode != http.StatusOK {
		return Identity{}, failure("upstream_error")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return Identity{}, &Error{Code: "unavailable", cause: ctx.Err()}
	}
	if len(body) > maxResponseSize {
		return Identity{}, failure("upstream_error")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return Identity{}, failure("upstream_error")
	}
	var result struct {
		OpenID     string  `json:"openid"`
		SessionKey string  `json:"session_key"`
		UnionID    *string `json:"unionid"`
		ErrCode    int     `json:"errcode"`
		ErrMsg     string  `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Identity{}, failure("upstream_error")
	}
	// 已知字段出现 null 属于协议错误；未知扩展字段不参与认证判断。
	for _, name := range []string{"openid", "session_key", "unionid", "errcode", "errmsg"} {
		if value, exists := fields[name]; exists && string(value) == "null" {
			return Identity{}, failure("upstream_error")
		}
	}
	switch result.ErrCode {
	case 0:
	case 40029, 40163:
		return Identity{}, failure("invalid_code")
	case 40226:
		return Identity{}, failure("blocked")
	case 45011:
		return Identity{}, failure("rate_limited")
	case -1:
		return Identity{}, failure("unavailable")
	default:
		return Identity{}, failure("upstream_error")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(result.SessionKey)
	if !validIdentity(result.OpenID) || err != nil || len(key) != 16 || (result.UnionID != nil && !validIdentity(*result.UnionID)) {
		return Identity{}, failure("upstream_error")
	}
	return Identity{AppID: credentials.AppID, OpenID: result.OpenID}, nil
}
