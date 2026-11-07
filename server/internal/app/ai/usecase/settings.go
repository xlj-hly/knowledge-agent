package usecase

import (
	"context"
	"errors"
	"go-server/internal/app/ai/dto"
	"go-server/internal/domain/ai"
)

func settingsView(s ai.Settings) dto.Settings {
	return dto.Settings{ChatModelID: s.ChatModelID, EmbeddingModelID: s.EmbeddingModelID, VectorStoreID: s.VectorStoreID, SystemPrompt: s.SystemPrompt, Temperature: s.Temperature, MaxTokens: s.MaxTokens, HistoryTurns: s.HistoryTurns, RetrievalMode: s.RetrievalMode, RetrievalLimit: s.RetrievalLimit, SimilarityThreshold: s.SimilarityThreshold, ChunkStrategy: s.ChunkStrategy, ChunkSize: s.ChunkSize, ChunkOverlap: s.ChunkOverlap, ParserEndpoint: s.ParserEndpoint}
}
func (s *Service) Settings(ctx context.Context, id dto.Identity) (dto.Settings, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Settings{}, err
	}
	v, err := s.Defaults(ctx, a.TenantID)
	return settingsView(v), err
}
func (s *Service) SaveSettings(ctx context.Context, id dto.Identity, in dto.SettingsInput) (dto.Settings, error) {
	return configured(ctx, s, id, func(ctx context.Context) (dto.Settings, error) { return s.saveSettings(ctx, id, in) })
}
func (s *Service) saveSettings(ctx context.Context, id dto.Identity, in dto.SettingsInput) (dto.Settings, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.Settings{}, err
	}
	current, err := s.Defaults(ctx, a.TenantID)
	if err != nil {
		return dto.Settings{}, err
	}
	// 与启用操作共用企业配置锁，只更新模板时保留锁内读取的当前模型。
	v := ai.Settings{TenantID: a.TenantID, ChatModelID: current.ChatModelID, EmbeddingModelID: in.EmbeddingModelID, VectorStoreID: in.VectorStoreID, SystemPrompt: in.SystemPrompt, Temperature: in.Temperature, MaxTokens: in.MaxTokens, HistoryTurns: in.HistoryTurns, RetrievalMode: in.RetrievalMode, RetrievalLimit: in.RetrievalLimit, SimilarityThreshold: in.SimilarityThreshold, ChunkStrategy: in.ChunkStrategy, ChunkSize: in.ChunkSize, ChunkOverlap: in.ChunkOverlap, ParserEndpoint: in.ParserEndpoint}
	if err = v.Validate(); err != nil {
		return dto.Settings{}, err
	}
	for _, ref := range []struct{ id, kind string }{{v.ChatModelID, "chat"}, {v.EmbeddingModelID, "embedding"}} {
		if ref.id != "" {
			if _, err = s.ResolveModel(ctx, a.TenantID, ref.id, ref.kind); err != nil {
				return dto.Settings{}, err
			}
		}
	}
	if v.VectorStoreID != "" && s.deps.References != nil {
		if err = s.deps.References.VectorStoreExists(ctx, a.TenantID, v.VectorStoreID); err != nil {
			return dto.Settings{}, err
		}
	}
	err = s.deps.Repository.SaveSettings(ctx, v)
	return settingsView(v), err
}

// ActivateModel 只切换本企业当前启用的 LLM，不修改其他模板或正在生成的请求。
func (s *Service) ActivateModel(ctx context.Context, id dto.Identity, target string) (dto.Settings, error) {
	return configured(ctx, s, id, func(ctx context.Context) (dto.Settings, error) {
		a, err := s.actor(ctx, id, true)
		if err != nil {
			return dto.Settings{}, err
		}
		if _, err = s.ResolveModel(ctx, a.TenantID, target, "chat"); err != nil {
			return dto.Settings{}, err
		}
		v, err := s.Defaults(ctx, a.TenantID)
		if err != nil {
			return dto.Settings{}, err
		}
		v.ChatModelID = target
		if err = s.deps.Repository.SaveSettings(ctx, v); err != nil {
			return dto.Settings{}, err
		}
		return settingsView(v), nil
	})
}
func (s *Service) Presets(ctx context.Context, id dto.Identity) (dto.Presets, error) {
	if _, err := s.actor(ctx, id, false); err != nil {
		return dto.Presets{}, err
	}
	if s.deps.Presets == nil {
		return dto.Presets{}, errors.New("AI 产品预设目录未配置")
	}
	return s.deps.Presets.Presets(), nil
}
