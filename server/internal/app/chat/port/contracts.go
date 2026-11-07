// Package port 定义聊天用例实际消费的授权、模型、知识检索与持久化契约。
package port

import (
	"context"
	"time"

	knowledgedto "go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/ai"
	"go-server/internal/domain/chat"
)

// Actor 只能来自 SaaS 会话验证。
type Actor struct{ TenantID, UserID, Role string }

// Authorizer 验证域名与令牌对应的当前企业身份。
type Authorizer interface {
	Authorize(context.Context, string, string) (Actor, error)
}

// Models 通过 AI 公开能力读取本企业模型和创建模板。
type Models interface {
	ResolveModel(context.Context, string, string, string) (ai.Model, error)
	Defaults(context.Context, string) (ai.Settings, error)
}

// Knowledge 使用知识库公开用例，不直接读取其表或索引。
type Knowledge interface {
	Base(context.Context, knowledgedto.Identity, string) (knowledgedto.Base, error)
	Search(context.Context, knowledgedto.Identity, string, knowledgedto.SearchInput) ([]knowledgedto.Hit, error)
}

// ChatTurn 是发往模型的有界上下文。
type ChatTurn struct{ Role, Content string }

// ChatRequest 明确模型本次生成参数。
type ChatRequest struct {
	Messages    []ChatTurn
	Temperature float64
	MaxTokens   int
}

// ChatModel 回调必须串行；回调失败或上下文取消时终止上游读取。
type ChatModel interface {
	Stream(context.Context, ai.Model, ChatRequest, func(string) error) error
}

// Repository 将生成领取与最终状态写入作为专门操作，保证同一会话只产生一条进行中的回答。
type Repository interface {
	Sessions(context.Context, string, string) ([]chat.Session, error)
	Session(context.Context, string, string, string) (chat.Session, error)
	SaveSession(context.Context, chat.Session, bool) error
	DeleteSession(context.Context, string, string, string) error
	Messages(context.Context, string, string, string) ([]chat.Message, error)
	Begin(context.Context, string, string, string, string, chat.Message, chat.Message) (chat.Session, []chat.Message, error)
	Progress(context.Context, chat.Message) error
	Finish(context.Context, chat.Message) (chat.Message, error)
	Cancel(context.Context, string, string, string) (bool, error)
	Recover(context.Context, time.Time) error
}
