package knowledge

import (
	domain "go-server/internal/domain/saas"
	"net/url"
	"strings"
	"unicode/utf8"
)

// ValidateEndpoint 允许管理员配置本地或远程 HTTP 服务，拒绝在地址中携带凭据。
func ValidateEndpoint(value string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || len(value) > 2048 || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return domain.Invalid("服务地址必须为不含凭据、查询参数的 HTTP(S) 地址")
	}
	return nil
}

// Validate 检查连接记录的共性结构；实际引擎及其协议限制由注入的目录校验。
func (v VectorStore) Validate() error {
	if strings.ContainsRune(v.Name, 0) {
		return domain.Invalid("向量连接配置不能包含 NUL 字符")
	}
	if err := domain.ValidateName(v.Name, 128, "连接名称"); err != nil {
		return err
	}
	if strings.TrimSpace(v.Provider) == "" || len(v.Provider) > 64 || strings.ContainsRune(v.Provider, 0) {
		return domain.Invalid("向量引擎标识无效")
	}
	if v.Config == nil || len(v.Config) > 64 {
		return domain.Invalid("向量连接配置必须为有界对象")
	}
	for key, value := range v.Config {
		if key == "" || len(key) > 64 || len(value) > 8192 || !utf8.ValidString(value) || strings.ContainsRune(key, 0) || strings.ContainsRune(value, 0) {
			return domain.Invalid("向量连接配置字段无效")
		}
	}
	return nil
}

// Validate 检查知识库处理配置，不允许向量模式隐式退化为关键词检索。
func (b Base) Validate() error {
	if strings.ContainsRune(b.Name, 0) || strings.ContainsRune(b.Description, 0) {
		return domain.Invalid("知识库名称和描述不能包含 NUL 字符")
	}
	if err := domain.ValidateName(b.Name, 128, "知识库名称"); err != nil {
		return err
	}
	if !utf8.ValidString(b.Description) || utf8.RuneCountInString(b.Description) > 2000 {
		return domain.Invalid("描述最多 2000 字")
	}
	if b.IndexingMode != "keyword" && b.IndexingMode != "hybrid" {
		return domain.Invalid("索引模式无效")
	}
	if b.IndexingMode != "keyword" && (b.EmbeddingModelID == "" || b.VectorStoreID == "") {
		return domain.Invalid("请先在页面配置并选择 Embedding 模型和向量连接")
	}
	if b.RetrievalMode != "keyword" && b.RetrievalMode != "vector" && b.RetrievalMode != "hybrid" {
		return domain.Invalid("检索策略无效")
	}
	if b.IndexingMode == "keyword" && b.RetrievalMode != "keyword" {
		return domain.Invalid("关键词索引只支持关键词检索；语义检索需要重建混合索引")
	}
	if err := b.Chunking().Validate(); err != nil {
		return err
	}
	return ValidateEndpoint(b.ParserEndpoint, true)
}

// Validate 校验实际支持的切分策略和字符长度。
func (c ChunkingConfig) Validate() error {
	switch c.Strategy {
	case "auto", "heading", "recursive":
	default:
		return domain.Invalid("文本分割策略无效")
	}
	if c.Size < 64 || c.Size > 8192 || c.Overlap < 0 || c.Overlap >= c.Size {
		return domain.Invalid("分块长度为64–8192，重叠必须小于分块长度")
	}
	return nil
}
