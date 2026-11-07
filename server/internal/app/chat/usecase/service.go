// Package usecase 编排会话、知识检索与模型生成，网络调用不持有数据库事务。
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"strconv"
	"sync"
	"time"

	"go-server/internal/app/chat/dto"
	"go-server/internal/app/chat/port"
	"go-server/internal/app/configuration"
	"go-server/internal/domain/chat"
	domain "go-server/internal/domain/saas"
)

// Dependencies 由 bootstrap 绑定公开能力和存储实现。
type Dependencies struct {
	Repository     port.Repository
	Authorizer     port.Authorizer
	Models         port.Models
	Knowledge      port.Knowledge
	Model          port.ChatModel
	Configurations configuration.Guard
}

// Service 仅管理本进程的取消句柄，消息和生成资格由数据库维护。
type Service struct {
	deps   Dependencies
	mu     sync.Mutex
	active map[string]context.CancelFunc
}

// New 不启动任务或读取外部配置。
func New(deps Dependencies) *Service {
	return &Service{deps: deps, active: map[string]context.CancelFunc{}}
}
func (s *Service) actor(ctx context.Context, id dto.Identity) (port.Actor, error) {
	return s.deps.Authorizer.Authorize(ctx, id.Hostname, id.Token)
}
func (s *Service) configuration(ctx context.Context, tenant string, fn func(context.Context) error) error {
	if s.deps.Configurations == nil {
		return fn(ctx)
	}
	return s.deps.Configurations.Within(ctx, tenant, fn)
}
func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	id := binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff
	if id == 0 {
		id = 1
	}
	return strconv.FormatUint(id, 10), nil
}
func validID(id string) error { _, err := domain.ParseID(id); return err }
func sessionView(v chat.Session) dto.Session {
	ids := append([]string{}, v.KnowledgeBaseIDs...)
	return dto.Session{ID: v.ID, Title: v.Title, ModelID: v.ModelID, KnowledgeBaseIDs: ids, SystemPrompt: v.SystemPrompt, Temperature: v.Temperature, MaxTokens: v.MaxTokens, HistoryTurns: v.HistoryTurns, RetrievalMode: v.RetrievalMode, RetrievalLimit: v.RetrievalLimit, SimilarityThreshold: v.SimilarityThreshold, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func sourceView(sources []chat.Source) []dto.Source {
	out := make([]dto.Source, 0, len(sources))
	for _, v := range sources {
		out = append(out, dto.Source{BaseID: v.BaseID, DocumentID: v.DocumentID, ChunkID: v.ChunkID, DocumentTitle: v.DocumentTitle, Content: v.Content, Score: v.Score})
	}
	return out
}
func messageView(v chat.Message) dto.Message {
	return dto.Message{ID: v.ID, SessionID: v.SessionID, Role: v.Role, Content: v.Content, Status: v.Status, Sources: sourceView(v.Sources), CreatedAt: v.CreatedAt}
}

// Recover 在当前单服务启动时结束前一进程遗留的生成状态。
func (s *Service) Recover(ctx context.Context) error {
	return s.deps.Repository.Recover(ctx, time.Now().UTC())
}
