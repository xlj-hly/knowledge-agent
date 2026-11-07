package bootstrap

import (
	"context"

	"go-server/internal/app/configuration"
	knowledgeport "go-server/internal/app/knowledge/port"
	knowledgeusecase "go-server/internal/app/knowledge/usecase"
	"go-server/internal/app/saas/dto"
	"go-server/internal/app/storage"
	aidriver "go-server/internal/infra/ai"
	knowledgedriver "go-server/internal/infra/knowledge"
	knowledgerepository "go-server/internal/infra/persistence/knowledge"
	"gorm.io/gorm"
)

type sessionReader interface {
	Me(context.Context, dto.TenantIdentity) (dto.TenantSession, error)
}

// knowledgeAuthorizer 通过 SaaS 公开用例复用会话、域名和停用校验，不直接读取其数据表。
type knowledgeAuthorizer struct{ sessions sessionReader }

func (a knowledgeAuthorizer) Authorize(ctx context.Context, hostname, token string) (knowledgeport.Actor, error) {
	session, err := a.sessions.Me(ctx, dto.TenantIdentity{Hostname: hostname, Token: token})
	if err != nil {
		return knowledgeport.Actor{}, err
	}
	return knowledgeport.Actor{TenantID: session.Tenant.ID, UserID: session.User.ID, Role: session.User.Role}, nil
}

func assembleKnowledge(db *gorm.DB, sessions sessionReader, objects storage.Store, models knowledgeport.Models, guard configuration.Guard, registry *aidriver.Registry, vectors *knowledgedriver.Vectors) *knowledgeusecase.Service {
	return knowledgeusecase.New(knowledgeusecase.Dependencies{
		Repository:     knowledgerepository.New(db),
		Authorizer:     knowledgeAuthorizer{sessions: sessions},
		Objects:        objects,
		Processor:      knowledgedriver.NewProcessor(),
		Embedder:       aidriver.NewEmbedder(registry),
		Catalog:        vectors,
		Vectors:        vectors,
		Models:         models,
		Configurations: guard,
	})
}
