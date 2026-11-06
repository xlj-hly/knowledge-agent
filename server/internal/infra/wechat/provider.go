package wechat

import (
	"context"
	"errors"

	"go-server/internal/app/saas/port"
)

type wechatAdapter struct{ client *Client }

// NewProvider 将微信协议客户端适配为业务端口，不改变一次性 code 的调用语义。
func NewProvider(client *Client) port.WeChatProvider {
	return &wechatAdapter{client: client}
}

func (a *wechatAdapter) Exchange(ctx context.Context, credentials port.WeChatCredentials, code string) (port.WeChatIdentity, error) {
	identity, err := a.client.Exchange(ctx, Credentials{AppID: credentials.AppID, AppSecret: credentials.AppSecret}, code)
	if err == nil {
		return port.WeChatIdentity{AppID: identity.AppID, OpenID: identity.OpenID}, nil
	}
	var failure *Error
	if errors.As(err, &failure) {
		return port.WeChatIdentity{}, port.NewWeChatError(failure.Code, failure)
	}
	return port.WeChatIdentity{}, port.NewWeChatError("upstream_error", err)
}

var _ port.WeChatProvider = (*wechatAdapter)(nil)
