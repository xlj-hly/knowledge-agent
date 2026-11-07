// Package dto 定义模型配置的输入与读取结果。
package dto

import "time"

type Identity struct{ Hostname, Token string }
type OK struct {
	OK bool `json:"ok"`
}
type ModelInput struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Provider  string `json:"provider"`
	Endpoint  string `json:"endpoint"`
	ModelName string `json:"model_name"`
	Dimension int    `json:"dimension"`
	APIKey    string `json:"api_key"`
}
type Model struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Provider  string    `json:"provider"`
	Endpoint  string    `json:"endpoint"`
	ModelName string    `json:"model_name"`
	Dimension int       `json:"dimension"`
	APIKey    string    `json:"api_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type ModelOption struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Dimension int    `json:"dimension"`
}
type Provider struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Protocol           string        `json:"protocol"`
	Endpoint           string        `json:"endpoint"`
	Kinds              []string      `json:"kinds"`
	Models             []ModelOption `json:"models"`
	Description        string        `json:"description"`
	Logo               string        `json:"logo"`
	CredentialRequired bool          `json:"credential_required"`
	Discovery          bool          `json:"discovery"`
}
type Discovery struct {
	Models []ModelOption `json:"models"`
}
type TestResult struct {
	OK        bool `json:"ok"`
	Dimension int  `json:"dimension"`
}
type Settings struct {
	ChatModelID         string  `json:"chat_model_id"`
	EmbeddingModelID    string  `json:"embedding_model_id"`
	VectorStoreID       string  `json:"vector_store_id"`
	SystemPrompt        string  `json:"system_prompt"`
	Temperature         float64 `json:"temperature"`
	MaxTokens           int     `json:"max_tokens"`
	HistoryTurns        int     `json:"history_turns"`
	RetrievalMode       string  `json:"retrieval_mode"`
	RetrievalLimit      int     `json:"retrieval_limit"`
	SimilarityThreshold float64 `json:"similarity_threshold"`
	ChunkStrategy       string  `json:"chunk_strategy"`
	ChunkSize           int     `json:"chunk_size"`
	ChunkOverlap        int     `json:"chunk_overlap"`
	ParserEndpoint      string  `json:"parser_endpoint"`
}

// SettingsInput 只写入新建模板；当前对话模型由 ActivateModel 独立管理。
type SettingsInput struct {
	EmbeddingModelID    string  `json:"embedding_model_id"`
	VectorStoreID       string  `json:"vector_store_id"`
	SystemPrompt        string  `json:"system_prompt"`
	Temperature         float64 `json:"temperature"`
	MaxTokens           int     `json:"max_tokens"`
	HistoryTurns        int     `json:"history_turns"`
	RetrievalMode       string  `json:"retrieval_mode"`
	RetrievalLimit      int     `json:"retrieval_limit"`
	SimilarityThreshold float64 `json:"similarity_threshold"`
	ChunkStrategy       string  `json:"chunk_strategy"`
	ChunkSize           int     `json:"chunk_size"`
	ChunkOverlap        int     `json:"chunk_overlap"`
	ParserEndpoint      string  `json:"parser_endpoint"`
}
type PromptPreset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}
type ChunkPreset struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Strategy string `json:"strategy"`
	Size     int    `json:"size"`
	Overlap  int    `json:"overlap"`
}
type ChunkStrategy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Presets struct {
	Prompts         []PromptPreset  `json:"prompts"`
	Chunking        []ChunkPreset   `json:"chunking"`
	ChunkStrategies []ChunkStrategy `json:"chunk_strategies"`
}
