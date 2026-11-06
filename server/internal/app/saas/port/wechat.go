package port

import (
	"context"
	"errors"
)

// WeChatCredentials 是只供服务端交换身份的配置，不得进入公开响应或日志。
type WeChatCredentials struct{ AppID, AppSecret string }

// WeChatIdentity 是微信服务器验证后的身份，不包含 session_key。
type WeChatIdentity struct{ AppID, OpenID string }

// WeChatProvider 在事务外交换一次性 code，并遵守调用方上下文。
type WeChatProvider interface {
	Exchange(context.Context, WeChatCredentials, string) (WeChatIdentity, error)
}

// WeChatError 提供稳定的故障分类，只保留标准上下文取消原因。
// Code 为 invalid_code、blocked、rate_limited、unavailable 或 upstream_error。
type WeChatError struct {
	Code  string
	cause error
}

// NewWeChatError 丢弃可能含秘密的网络错误，仅保留可供 errors.Is 判断的取消语义。
func NewWeChatError(code string, cause error) *WeChatError {
	e := &WeChatError{Code: code}
	if errors.Is(cause, context.Canceled) {
		e.cause = context.Canceled
	} else if errors.Is(cause, context.DeadlineExceeded) {
		e.cause = context.DeadlineExceeded
	}
	return e
}

func (e *WeChatError) Error() string {
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

// Unwrap 仅返回标准上下文取消原因，不透传上游错误或请求内容。
func (e *WeChatError) Unwrap() error { return e.cause }
