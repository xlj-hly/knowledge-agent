// Package chat 定义会话配置、消息与来源快照；不依赖模型协议或 HTTP。
package chat

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	domain "go-server/internal/domain/saas"
)

// Session 的参数在创建时从企业模板复制；ModelID 是最近一次生成的模型快照，不是模型引用配置。
type Session struct {
	ID, TenantID, UserID, Title, ModelID, SystemPrompt string
	KnowledgeBaseIDs                                   []string
	Temperature, SimilarityThreshold                   float64
	MaxTokens, HistoryTurns, RetrievalLimit            int
	RetrievalMode                                      string
	CreatedAt, UpdatedAt                               time.Time
}

// Source 保存回答时实际使用的资料快照，原文后续变更不会改变历史回答的引用。
type Source struct {
	BaseID, DocumentID, ChunkID, DocumentTitle, Content string
	Score                                               float64
}

// Message 属于一个用户会话；Status 只允许通过生成生命周期修改。
type Message struct {
	ID, TenantID, UserID, SessionID, Role, Content, Status string
	Sources                                                []Source
	CreatedAt                                              time.Time
}

// Validate 校验完整会话配置；资源归属由应用用例向各能力确认。
func (s Session) Validate() error {
	if err := domain.ValidateName(s.Title, 100, "会话标题"); err != nil {
		return err
	}
	if s.ModelID != "" {
		if _, err := domain.ParseID(s.ModelID); err != nil {
			return err
		}
	}
	if len(s.KnowledgeBaseIDs) > 10 {
		return domain.Invalid("一次对话最多选择10个知识库")
	}
	seen := map[string]bool{}
	for _, id := range s.KnowledgeBaseIDs {
		if _, err := domain.ParseID(id); err != nil {
			return err
		}
		if seen[id] {
			return domain.Invalid("知识库不能重复选择")
		}
		seen[id] = true
	}
	if !utf8.ValidString(s.SystemPrompt) || strings.ContainsRune(s.SystemPrompt, 0) || len(s.SystemPrompt) > 16000 {
		return domain.Invalid("System Prompt 无效或超过16000字节")
	}
	if math.IsNaN(s.Temperature) || math.IsInf(s.Temperature, 0) || s.Temperature < 0 || s.Temperature > 2 {
		return domain.Invalid("Temperature 须为0–2")
	}
	if s.MaxTokens < 1 || s.MaxTokens > 32768 || s.HistoryTurns < 0 || s.HistoryTurns > 50 {
		return domain.Invalid("输出上限为1–32768，历史轮数为0–50")
	}
	if s.RetrievalMode != "keyword" && s.RetrievalMode != "vector" && s.RetrievalMode != "hybrid" {
		return domain.Invalid("检索方式无效")
	}
	if s.RetrievalLimit < 1 || s.RetrievalLimit > 20 || math.IsNaN(s.SimilarityThreshold) || math.IsInf(s.SimilarityThreshold, 0) || s.SimilarityThreshold < 0 || s.SimilarityThreshold > 1 {
		return domain.Invalid("检索数量为1–20，分数阈值为0–1")
	}
	return nil
}

// ValidateContent 限制单次用户输入；检索问题另行限制在知识库接口允许的长度内。
func ValidateContent(value string) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > 16000 {
		return domain.Invalid("消息不能为空且最多16000字")
	}
	return nil
}
