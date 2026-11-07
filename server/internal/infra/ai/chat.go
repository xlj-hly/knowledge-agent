package ai

import (
	"context"
	"errors"

	"go-server/internal/app/chat/port"
	domain "go-server/internal/domain/ai"
)

// Chat 解析注册的服务商能力，不承担会话、凭据或消息持久化。
type Chat struct{ registry *Registry }

// NewChat 使用组装阶段创建的只读能力目录。
func NewChat(registry *Registry) *Chat { return &Chat{registry: registry} }

// Stream 将业务请求交给服务商声明的协议驱动。
func (c *Chat) Stream(ctx context.Context, model domain.Model, in port.ChatRequest, emit func(string) error) error {
	if model.Kind != "chat" || model.ModelName == "" {
		return errors.New("对话模型配置无效")
	}
	definition, driver, err := c.registry.resolve(model.Provider, "chat")
	if err != nil {
		return err
	}
	return driver.Stream(ctx, definition, model, in, emit)
}
