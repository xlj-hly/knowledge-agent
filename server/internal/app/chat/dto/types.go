// Package dto 定义对话公开契约，凭据和内部生成控制不返回客户端。
package dto

import "time"

// Identity 使用已存在的企业登录态；请求体不能指定租户和用户。
type Identity struct{ Hostname, Token string }

// SessionInput 使用指针区分创建时缺省与显式零值；更新同样保留未提供字段。
type SessionInput struct {
	Title               *string   `json:"title"`
	KnowledgeBaseIDs    *[]string `json:"knowledge_base_ids"`
	SystemPrompt        *string   `json:"system_prompt"`
	Temperature         *float64  `json:"temperature"`
	MaxTokens           *int      `json:"max_tokens"`
	HistoryTurns        *int      `json:"history_turns"`
	RetrievalMode       *string   `json:"retrieval_mode"`
	RetrievalLimit      *int      `json:"retrieval_limit"`
	SimilarityThreshold *float64  `json:"similarity_threshold"`
}

// Session 提供会话配置及状态；ModelID 仅记录最后一次生成采用的模型快照。
type Session struct {
	ID                  string    `json:"id"`
	Title               string    `json:"title"`
	ModelID             string    `json:"model_id"`
	KnowledgeBaseIDs    []string  `json:"knowledge_base_ids"`
	SystemPrompt        string    `json:"system_prompt"`
	Temperature         float64   `json:"temperature"`
	MaxTokens           int       `json:"max_tokens"`
	HistoryTurns        int       `json:"history_turns"`
	RetrievalMode       string    `json:"retrieval_mode"`
	RetrievalLimit      int       `json:"retrieval_limit"`
	SimilarityThreshold float64   `json:"similarity_threshold"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Source 保存已授权检索命中的文本快照。
type Source struct {
	BaseID        string  `json:"base_id"`
	DocumentID    string  `json:"document_id"`
	ChunkID       string  `json:"chunk_id"`
	DocumentTitle string  `json:"document_title"`
	Content       string  `json:"content"`
	Score         float64 `json:"score"`
}

// Message 包括已完成、失败或取消后的持久化文本。
type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	Sources   []Source  `json:"sources"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageInput 提交一条新用户消息。
type MessageInput struct {
	Content string `json:"content"`
}

// OK 表示请求已完成。
type OK struct {
	OK bool `json:"ok"`
}

// Failure 是允许对用户公开的流式错误，不携带原始上游内容。
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Delta 是本次流式增量。
type Delta struct {
	Content string `json:"content"`
}

// Event 以 Type 确定唯一有效的负载，接入层负责 SSE 编码。
type Event struct {
	Type    string
	Message Message
	Sources []Source
	Delta   Delta
	Failure Failure
}
