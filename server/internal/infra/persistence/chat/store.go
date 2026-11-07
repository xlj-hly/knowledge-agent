// Package repository 持久化用户会话与消息，并以会话行锁协调生成生命周期。
package repository

import (
	"context"
	"errors"
	"time"

	"go-server/internal/app/chat/port"
	"go-server/internal/domain/chat"
	domain "go-server/internal/domain/saas"
	"go-server/internal/infra/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store 的数据库生命周期由 bootstrap 管理。
type Store struct{ db *gorm.DB }

func (s *Store) connection(ctx context.Context) *gorm.DB {
	return database.Connection(ctx, s.db).WithContext(ctx)
}

var _ port.Repository = (*Store)(nil)

// New 不自动建表或启动后台任务。
func New(db *gorm.DB) *Store { return &Store{db: db} }

type sessionRow struct {
	ID, TenantID, UserID, Title, SystemPrompt string
	ModelID                                   *string
	KnowledgeBaseIDs                          []string `gorm:"serializer:json;type:jsonb"`
	Temperature, SimilarityThreshold          float64
	MaxTokens, HistoryTurns, RetrievalLimit   int
	RetrievalMode                             string
	CreatedAt, UpdatedAt                      time.Time
}

func (sessionRow) TableName() string { return "chat_sessions" }

type messageRow struct {
	ID, TenantID, UserID, SessionID, Role, Content, Status string
	Sources                                                []chat.Source `gorm:"serializer:json;type:jsonb"`
	CreatedAt                                              time.Time
}

func (messageRow) TableName() string { return "chat_messages" }
func rowSession(s chat.Session) sessionRow {
	r := sessionRow{ID: s.ID, TenantID: s.TenantID, UserID: s.UserID, Title: s.Title, KnowledgeBaseIDs: s.KnowledgeBaseIDs, SystemPrompt: s.SystemPrompt, Temperature: s.Temperature, SimilarityThreshold: s.SimilarityThreshold, MaxTokens: s.MaxTokens, HistoryTurns: s.HistoryTurns, RetrievalLimit: s.RetrievalLimit, RetrievalMode: s.RetrievalMode, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	if s.ModelID != "" {
		r.ModelID = &s.ModelID
	}
	return r
}
func (r sessionRow) view() chat.Session {
	s := chat.Session{ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, Title: r.Title, KnowledgeBaseIDs: r.KnowledgeBaseIDs, SystemPrompt: r.SystemPrompt, Temperature: r.Temperature, SimilarityThreshold: r.SimilarityThreshold, MaxTokens: r.MaxTokens, HistoryTurns: r.HistoryTurns, RetrievalLimit: r.RetrievalLimit, RetrievalMode: r.RetrievalMode, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	if r.ModelID != nil {
		s.ModelID = *r.ModelID
	}
	if s.KnowledgeBaseIDs == nil {
		s.KnowledgeBaseIDs = []string{}
	}
	return s
}
func rowMessage(m chat.Message) messageRow { return messageRow(m) }
func (s *Store) scope(ctx context.Context, t, u string) *gorm.DB {
	return s.connection(ctx).Where("tenant_id = ? AND user_id = ?", t, u)
}

type storageError struct {
	operation string
	cause     error
}

func (e *storageError) Error() string     { return "chat persistence failed" }
func (e *storageError) Unwrap() error     { return e.cause }
func (e *storageError) Operation() string { return "chat " + e.operation }
func checked(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NotFound()
	}
	var business *domain.Error
	if errors.As(err, &business) {
		return err
	}
	return &storageError{op, err}
}

// Sessions 仅列出当前企业内当前用户的会话。
func (s *Store) Sessions(ctx context.Context, t, u string) ([]chat.Session, error) {
	rows := []sessionRow{}
	err := s.scope(ctx, t, u).Order("updated_at DESC,id DESC").Find(&rows).Error
	out := make([]chat.Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view())
	}
	return out, checked("list sessions", err)
}
func (s *Store) session(ctx context.Context, t, u, id string, lock bool) (chat.Session, error) {
	q := s.scope(ctx, t, u)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var r sessionRow
	err := q.First(&r, "id = ?", id).Error
	return r.view(), checked("get session", err)
}

// Session 将会话标识与用户、企业一起限定。
func (s *Store) Session(ctx context.Context, t, u, id string) (chat.Session, error) {
	return s.session(ctx, t, u, id, false)
}
func (s *Store) busy(ctx context.Context, t, u, id string) error {
	var n int64
	if err := s.scope(ctx, t, u).Model(&messageRow{}).Where("session_id = ? AND status = 'generating'", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return domain.Conflict("当前会话正在生成回答，请先停止或等待完成")
	}
	return nil
}

// SaveSession 更新前锁定会话，防止生成过程中改变本次请求配置。
func (s *Store) SaveSession(ctx context.Context, v chat.Session, create bool) error {
	r := rowSession(v)
	if create {
		return checked("create session", s.connection(ctx).Create(&r).Error)
	}
	return checked("update session", s.connection(ctx).Transaction(func(db *gorm.DB) error {
		tx := New(db)
		if _, err := tx.session(ctx, v.TenantID, v.UserID, v.ID, true); err != nil {
			return err
		}
		if err := tx.busy(ctx, v.TenantID, v.UserID, v.ID); err != nil {
			return err
		}
		return tx.scope(ctx, v.TenantID, v.UserID).Model(&sessionRow{}).Where("id = ?", v.ID).Select("Title", "KnowledgeBaseIDs", "SystemPrompt", "Temperature", "SimilarityThreshold", "MaxTokens", "HistoryTurns", "RetrievalLimit", "RetrievalMode", "UpdatedAt").Updates(&r).Error
	}))
}

// DeleteSession 与消息删除在同一事务中，进行中的生成必须先停止。
func (s *Store) DeleteSession(ctx context.Context, t, u, id string) error {
	return checked("delete session", s.connection(ctx).Transaction(func(db *gorm.DB) error {
		tx := New(db)
		if _, err := tx.session(ctx, t, u, id, true); err != nil {
			return err
		}
		if err := tx.busy(ctx, t, u, id); err != nil {
			return err
		}
		if err := tx.scope(ctx, t, u).Delete(&messageRow{}, "session_id = ?", id).Error; err != nil {
			return err
		}
		return tx.scope(ctx, t, u).Delete(&sessionRow{}, "id = ?", id).Error
	}))
}

// Messages 返回时间顺序的消息，先检查会话归属，避免外部会话标识泄露。
func (s *Store) Messages(ctx context.Context, t, u, id string) ([]chat.Message, error) {
	if _, err := s.Session(ctx, t, u, id); err != nil {
		return nil, err
	}
	rows := []messageRow{}
	err := s.scope(ctx, t, u).Where("session_id = ?", id).Order("created_at ASC,id ASC").Find(&rows).Error
	out := make([]chat.Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, chat.Message(r))
	}
	return out, checked("list messages", err)
}

// Begin 在短事务内取得生成资格并写入问答消息；网络调用始终在事务外执行。
func (s *Store) Begin(ctx context.Context, t, u, id, modelID string, user, assistant chat.Message) (chat.Session, []chat.Message, error) {
	var session chat.Session
	history := []chat.Message{}
	err := s.connection(ctx).Transaction(func(db *gorm.DB) error {
		tx := New(db)
		var err error
		session, err = tx.session(ctx, t, u, id, true)
		if err != nil {
			return err
		}
		if err = tx.busy(ctx, t, u, id); err != nil {
			return err
		}
		session.ModelID = modelID
		rows := []messageRow{}
		if err = tx.scope(ctx, t, u).Where("session_id = ?", id).Order("created_at DESC,id DESC").Limit(session.HistoryTurns*4 + 4).Find(&rows).Error; err != nil {
			return err
		}
		for i := len(rows) - 1; i >= 0; i-- {
			history = append(history, chat.Message(rows[i]))
		}
		ur, ar := rowMessage(user), rowMessage(assistant)
		if err = db.Create(&ur).Error; err != nil {
			return err
		}
		if err = db.Create(&ar).Error; err != nil {
			return err
		}
		return tx.scope(ctx, t, u).Model(&sessionRow{}).Where("id = ?", id).Updates(map[string]any{"updated_at": assistant.CreatedAt, "model_id": modelID}).Error
	})
	return session, history, checked("begin generation", err)
}

// Progress 以状态 CAS 保存部分回答；停止后到达的旧增量不能恢复 generating。
func (s *Store) Progress(ctx context.Context, m chat.Message) error {
	r := rowMessage(m)
	result := s.scope(ctx, m.TenantID, m.UserID).Model(&messageRow{}).Where("id = ? AND session_id = ? AND status = 'generating'", m.ID, m.SessionID).Select("Content", "Sources").Updates(&r)
	if result.Error != nil {
		return checked("save progress", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Conflict("本次回答已停止")
	}
	return nil
}

// Finish 在取消与正常完成竞争时保留已提交的取消状态，同时保存取消前的部分文本。
func (s *Store) Finish(ctx context.Context, m chat.Message) (chat.Message, error) {
	var out chat.Message
	err := s.connection(ctx).Transaction(func(db *gorm.DB) error {
		tx := New(db)
		if _, err := tx.session(ctx, m.TenantID, m.UserID, m.SessionID, true); err != nil {
			return err
		}
		var current messageRow
		if err := tx.scope(ctx, m.TenantID, m.UserID).First(&current, "id = ? AND session_id = ?", m.ID, m.SessionID).Error; err != nil {
			return err
		}
		if current.Status != "generating" && current.Status != "cancelled" {
			out = chat.Message(current)
			return nil
		}
		if current.Status == "cancelled" {
			m.Status = "cancelled"
		}
		r := rowMessage(m)
		if err := tx.scope(ctx, m.TenantID, m.UserID).Model(&messageRow{}).Where("id = ?", m.ID).Select("Content", "Status", "Sources").Updates(&r).Error; err != nil {
			return err
		}
		out = m
		return tx.scope(ctx, m.TenantID, m.UserID).Model(&sessionRow{}).Where("id = ?", m.SessionID).Update("updated_at", time.Now().UTC()).Error
	})
	return out, checked("finish generation", err)
}

// Cancel 先持久化取消，断线客户端随后重新读取也能确认停止结果。
func (s *Store) Cancel(ctx context.Context, t, u, id string) (bool, error) {
	cancelled := false
	err := s.connection(ctx).Transaction(func(db *gorm.DB) error {
		tx := New(db)
		if _, err := tx.session(ctx, t, u, id, true); err != nil {
			return err
		}
		result := tx.scope(ctx, t, u).Model(&messageRow{}).Where("session_id = ? AND status = 'generating'", id).Update("status", "cancelled")
		cancelled = result.RowsAffected > 0
		return result.Error
	})
	return cancelled, checked("cancel generation", err)
}

// Recover 由单个服务进程启动时调用，结束前一进程留下的生成，避免永久阻塞会话。
func (s *Store) Recover(ctx context.Context, before time.Time) error {
	return checked("recover generation", s.connection(ctx).Model(&messageRow{}).Where("status = 'generating' AND created_at <= ?", before).Update("status", "failed").Error)
}
