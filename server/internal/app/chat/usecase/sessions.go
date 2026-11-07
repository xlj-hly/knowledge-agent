package usecase

import (
	"context"
	"strings"
	"time"

	"go-server/internal/app/chat/dto"
	kdto "go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/chat"
)

// Sessions 返回当前用户自己的会话，其他成员不共享消息历史。
func (s *Service) Sessions(ctx context.Context, id dto.Identity) ([]dto.Session, error) {
	a, err := s.actor(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Repository.Sessions(ctx, a.TenantID, a.UserID)
	out := make([]dto.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionView(row))
	}
	return out, err
}

// Session 读取当前用户的会话配置。
func (s *Service) Session(ctx context.Context, id dto.Identity, target string) (dto.Session, error) {
	a, err := s.actor(ctx, id)
	if err != nil {
		return dto.Session{}, err
	}
	if err = validID(target); err != nil {
		return dto.Session{}, err
	}
	row, err := s.deps.Repository.Session(ctx, a.TenantID, a.UserID, target)
	return sessionView(row), err
}

// SaveSession 在创建时复制企业参数模板；每次生成的模型由企业当前启用值决定。
func (s *Service) SaveSession(ctx context.Context, id dto.Identity, target string, in dto.SessionInput) (dto.Session, error) {
	a, err := s.actor(ctx, id)
	if err != nil {
		return dto.Session{}, err
	}
	create := target == ""
	if !create {
		if err = validID(target); err != nil {
			return dto.Session{}, err
		}
	}
	var row chat.Session
	err = s.configuration(ctx, a.TenantID, func(ctx context.Context) error {
		if create {
			defaults, e := s.deps.Models.Defaults(ctx, a.TenantID)
			if e != nil {
				return e
			}
			newid, e := newID()
			if e != nil {
				return e
			}
			stamp := time.Now().UTC()
			row = chat.Session{ID: newid, TenantID: a.TenantID, UserID: a.UserID, Title: "新对话", KnowledgeBaseIDs: []string{}, SystemPrompt: defaults.SystemPrompt, Temperature: defaults.Temperature, MaxTokens: defaults.MaxTokens, HistoryTurns: defaults.HistoryTurns, RetrievalMode: defaults.RetrievalMode, RetrievalLimit: defaults.RetrievalLimit, SimilarityThreshold: defaults.SimilarityThreshold, CreatedAt: stamp, UpdatedAt: stamp}
		} else {
			var e error
			row, e = s.deps.Repository.Session(ctx, a.TenantID, a.UserID, target)
			if e != nil {
				return e
			}
			row.UpdatedAt = time.Now().UTC()
		}
		applyInput(&row, in)
		if e := row.Validate(); e != nil {
			return e
		}
		for _, base := range row.KnowledgeBaseIDs {
			if _, e := s.deps.Knowledge.Base(ctx, kdto.Identity{Hostname: id.Hostname, Token: id.Token}, base); e != nil {
				return e
			}
		}
		return s.deps.Repository.SaveSession(ctx, row, create)
	})
	return sessionView(row), err
}
func applyInput(row *chat.Session, in dto.SessionInput) {
	if in.Title != nil {
		row.Title = strings.TrimSpace(*in.Title)
	}
	if in.KnowledgeBaseIDs != nil {
		row.KnowledgeBaseIDs = append([]string{}, (*in.KnowledgeBaseIDs)...)
	}
	if in.SystemPrompt != nil {
		row.SystemPrompt = *in.SystemPrompt
	}
	if in.Temperature != nil {
		row.Temperature = *in.Temperature
	}
	if in.MaxTokens != nil {
		row.MaxTokens = *in.MaxTokens
	}
	if in.HistoryTurns != nil {
		row.HistoryTurns = *in.HistoryTurns
	}
	if in.RetrievalMode != nil {
		row.RetrievalMode = *in.RetrievalMode
	}
	if in.RetrievalLimit != nil {
		row.RetrievalLimit = *in.RetrievalLimit
	}
	if in.SimilarityThreshold != nil {
		row.SimilarityThreshold = *in.SimilarityThreshold
	}
}

// DeleteSession 原子删除本用户会话及消息；正在生成时要求先停止。
func (s *Service) DeleteSession(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	a, err := s.actor(ctx, id)
	if err != nil {
		return dto.OK{}, err
	}
	if err = validID(target); err != nil {
		return dto.OK{}, err
	}
	err = s.deps.Repository.DeleteSession(ctx, a.TenantID, a.UserID, target)
	return dto.OK{OK: err == nil}, err
}

// Messages 读取消息及其生成时引用快照，重载不依赖当前资料是否仍存在。
func (s *Service) Messages(ctx context.Context, id dto.Identity, target string) ([]dto.Message, error) {
	a, err := s.actor(ctx, id)
	if err != nil {
		return nil, err
	}
	if err = validID(target); err != nil {
		return nil, err
	}
	rows, err := s.deps.Repository.Messages(ctx, a.TenantID, a.UserID, target)
	out := make([]dto.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, messageView(row))
	}
	return out, err
}

// Stop 持久化取消并结束本进程上游请求，其他用户不能控制此会话。
func (s *Service) Stop(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	a, err := s.actor(ctx, id)
	if err != nil {
		return dto.OK{}, err
	}
	if err = validID(target); err != nil {
		return dto.OK{}, err
	}
	if _, err = s.deps.Repository.Cancel(ctx, a.TenantID, a.UserID, target); err != nil {
		return dto.OK{}, err
	}
	s.mu.Lock()
	cancel := s.active[target]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return dto.OK{OK: true}, nil
}
