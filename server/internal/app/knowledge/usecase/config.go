package usecase

import (
	"context"
	"errors"
	"go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/knowledge"
	domain "go-server/internal/domain/saas"
	"maps"
	"strings"
)

// VectorProviders 只公开已实现引擎的静态能力目录，企业身份仍由会话校验。
func (s *Service) VectorProviders(ctx context.Context, id dto.Identity) ([]dto.VectorProvider, error) {
	if _, err := s.actor(ctx, id, false); err != nil {
		return nil, err
	}
	if s.deps.Catalog == nil {
		return nil, errors.New("向量引擎目录未组装")
	}
	return s.deps.Catalog.Providers(), nil
}

// VectorStores 返回本企业可选向量服务。
func (s *Service) VectorStores(ctx context.Context, id dto.Identity) ([]dto.VectorStore, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Repository.VectorStores(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	defaults, err := s.deps.Models.Defaults(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.VectorStore, 0, len(rows))
	for _, row := range rows {
		view := vectorStoreView(row)
		references, e := s.deps.Repository.VectorStoreReferences(ctx, a.TenantID, row.ID)
		if e != nil {
			return nil, e
		}
		view.Referenced = references > 0 || defaults.VectorStoreID == row.ID
		out = append(out, view)
	}
	return out, nil
}

func vectorStoreView(store knowledge.VectorStore) dto.VectorStore {
	return dto.VectorStore{ID: store.ID, Name: store.Name, Provider: store.Provider, Config: maps.Clone(store.Config), CreatedAt: store.CreatedAt, UpdatedAt: store.UpdatedAt}
}

// SaveVectorStore 完整替换连接配置；选填凭据留空或省略时清除原值。
func (s *Service) SaveVectorStore(ctx context.Context, id dto.Identity, target string, in dto.VectorStoreInput) (dto.VectorStore, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.VectorStore{}, err
	}
	create := target == ""
	if create {
		target, err = newID()
	} else {
		err = validID(target)
	}
	if err != nil {
		return dto.VectorStore{}, err
	}
	candidate := knowledge.VectorStore{ID: target, TenantID: a.TenantID, Name: strings.TrimSpace(in.Name), Provider: in.Provider, Config: in.Config, CreatedAt: now(), UpdatedAt: now()}
	if s.deps.Catalog == nil {
		return dto.VectorStore{}, errors.New("向量引擎目录未组装")
	}
	if err := s.deps.Catalog.ValidateInput(candidate); err != nil {
		return dto.VectorStore{}, err
	}
	candidate, err = s.deps.Catalog.PrepareStore(candidate)
	if err != nil {
		return dto.VectorStore{}, err
	}
	referenced := false
	err = s.withConfiguration(ctx, a.TenantID, func(ctx context.Context) error {
		defaults, e := s.deps.Models.Defaults(ctx, a.TenantID)
		if e != nil {
			return e
		}
		return s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
			if !create {
				stored, e := tx.VectorStore(ctx, a.TenantID, target, true)
				if e != nil {
					return e
				}
				n, e := tx.VectorStoreReferences(ctx, a.TenantID, target)
				if e != nil {
					return e
				}
				referenced = n > 0 || defaults.VectorStoreID == target
				candidate.CreatedAt = stored.CreatedAt
				if referenced && s.deps.Catalog.ImmutableChanged(stored, candidate) {
					return domain.Conflict("向量连接已被知识库使用，请新建另一连接")
				}
			}
			return tx.SaveVectorStore(ctx, candidate, create)
		})
	})
	if err != nil {
		return dto.VectorStore{}, err
	}
	view := vectorStoreView(candidate)
	view.Referenced = referenced
	return view, nil
}

// DeleteVectorStore 删除没有知识库引用的连接。
func (s *Service) DeleteVectorStore(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.OK{}, err
	}
	if err = validID(target); err != nil {
		return dto.OK{}, err
	}
	err = s.withConfiguration(ctx, a.TenantID, func(ctx context.Context) error {
		defaults, err := s.deps.Models.Defaults(ctx, a.TenantID)
		if err != nil {
			return err
		}
		if defaults.VectorStoreID == target {
			return domain.Conflict("请先更改企业默认向量连接")
		}
		return s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
			if _, e := tx.VectorStore(ctx, a.TenantID, target, true); e != nil {
				return e
			}
			n, e := tx.VectorStoreReferences(ctx, a.TenantID, target)
			if e != nil {
				return e
			}
			if n > 0 {
				return domain.Conflict("请先删除引用此连接的知识库")
			}
			return tx.DeleteVectorStore(ctx, a.TenantID, target)
		})
	})
	return dto.OK{OK: err == nil}, err
}

// TestVectorStore 验证页面保存的向量连接。
func (s *Service) TestVectorStore(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.OK{}, err
	}
	if err = validID(target); err != nil {
		return dto.OK{}, err
	}
	m, err := s.deps.Repository.VectorStore(ctx, a.TenantID, target, false)
	if err != nil {
		return dto.OK{}, err
	}
	err = s.deps.Vectors.Test(ctx, m)
	if err != nil {
		return dto.OK{}, connectionFailure("vector connection test", "向量连接测试失败，请检查服务地址及访问凭据", err)
	}
	return dto.OK{OK: err == nil}, err
}

// connectionError 保留外部错误链供受控诊断；公开提示不携带上游响应或连接凭据。
type connectionError struct {
	operation, message string
	cause              error
}

func (e *connectionError) Error() string { return "knowledge_upstream_error" }
func (e *connectionError) Unwrap() []error {
	return []error{&domain.Error{Code: "knowledge_upstream_error", Message: e.message}, e.cause}
}
func (e *connectionError) Operation() string { return e.operation }
func connectionFailure(operation, message string, cause error) error {
	return &connectionError{operation: operation, message: message, cause: cause}
}
