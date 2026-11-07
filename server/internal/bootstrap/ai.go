package bootstrap

import (
	"context"
	aiport "go-server/internal/app/ai/port"
	aiusecase "go-server/internal/app/ai/usecase"
	chatport "go-server/internal/app/chat/port"
	chatusecase "go-server/internal/app/chat/usecase"
	"go-server/internal/app/configuration"
	knowledgeusecase "go-server/internal/app/knowledge/usecase"
	"go-server/internal/app/saas/dto"
	aidriver "go-server/internal/infra/ai"
	airepository "go-server/internal/infra/persistence/ai"
	chatrepository "go-server/internal/infra/persistence/chat"
	"gorm.io/gorm"
)

type aiAuthorizer struct{ sessions sessionReader }

func (a aiAuthorizer) Authorize(ctx context.Context, hostname, token string) (aiport.Actor, error) {
	v, err := a.sessions.Me(ctx, dto.TenantIdentity{Hostname: hostname, Token: token})
	if err != nil {
		return aiport.Actor{}, err
	}
	return aiport.Actor{TenantID: v.Tenant.ID, UserID: v.User.ID, Role: v.User.Role}, nil
}

type chatAuthorizer struct{ sessions sessionReader }

func (a chatAuthorizer) Authorize(ctx context.Context, hostname, token string) (chatport.Actor, error) {
	v, err := a.sessions.Me(ctx, dto.TenantIdentity{Hostname: hostname, Token: token})
	if err != nil {
		return chatport.Actor{}, err
	}
	return chatport.Actor{TenantID: v.Tenant.ID, UserID: v.User.ID, Role: v.User.Role}, nil
}

// 配置引用只通过业务公开能力校验；这些指针在组装结束前固定，不在运行时改写。
type aiReferences struct {
	knowledge *knowledgeusecase.Service
}

func (r *aiReferences) ModelReferences(ctx context.Context, t, id string) (int64, error) {
	return r.knowledge.ModelReferences(ctx, t, id)
}
func (r *aiReferences) VectorStoreExists(ctx context.Context, t, id string) error {
	return r.knowledge.ResolveVectorStore(ctx, t, id)
}
func assembleAI(db *gorm.DB, sessions sessionReader, refs *aiReferences, guard configuration.Guard, registry *aidriver.Registry, presets aiport.PresetCatalog) *aiusecase.Service {
	return aiusecase.New(aiusecase.Dependencies{Repository: airepository.New(db), Authorizer: aiAuthorizer{sessions}, Catalog: registry, Presets: presets, Probe: aidriver.NewProbe(registry), References: refs, Configurations: guard})
}
func assembleChat(db *gorm.DB, sessions sessionReader, models chatport.Models, knowledge chatport.Knowledge, guard configuration.Guard, registry *aidriver.Registry) *chatusecase.Service {
	return chatusecase.New(chatusecase.Dependencies{Repository: chatrepository.New(db), Authorizer: chatAuthorizer{sessions}, Models: models, Knowledge: knowledge, Model: aidriver.NewChat(registry), Configurations: guard})
}
