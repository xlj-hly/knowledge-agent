package knowledge

import (
	_ "embed"
	"encoding/base64"
	"strings"

	"go-server/internal/app/knowledge/dto"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
)

//go:embed assets/qdrant.png
var qdrantLogo []byte

// NewBuiltinVectors 显式组装本程序实现的引擎，不借助全局注册或隐式默认驱动。
func NewBuiltinVectors() (*Vectors, error) {
	entries := []Entry{qdrantEntry(), PGVectorEntry()}
	entries = append(entries, ChromaEntries()...)
	entries = append(entries, PineconeEntry())
	entries = append(entries, MilvusEntries()...)
	entries = append(entries, WeaviateEntry(), AstraEntry())
	entries = append(entries, ElasticsearchEntries()...)
	entries = append(entries, DorisEntry(), TencentVectorDBEntry())
	return NewVectors(entries...)
}

func qdrantEntry() Entry {
	return Entry{
		Definition: dto.VectorProvider{
			ID: "qdrant", Name: "Qdrant", Description: "连接自建实例或 Qdrant Cloud · HTTP API",
			Logo:     "data:image/png;base64," + base64.StdEncoding.EncodeToString(qdrantLogo),
			Protocol: "qdrant", Fields: []dto.VectorField{
				{Key: "endpoint", Label: "服务地址", Type: "text", Required: true, Immutable: true, Default: "http://host.docker.internal:6333", Help: "实例 HTTP(S) 地址；同机 Docker 服务可用 host.docker.internal。"},
				{Key: "api_key", Label: "API Key", Type: "password", Secret: true},
				{Key: "collection", Label: "集合名称", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge", Help: "本应用索引前缀，按向量维度创建集合。"},
			},
		},
		Driver: newQdrantDriver(), Validate: validateQdrantStore,
		Normalize: func(config map[string]string) (map[string]string, error) {
			config = cloneConfig(config)
			config["endpoint"] = strings.TrimRight(strings.TrimSpace(config["endpoint"]), "/")
			config["collection"] = strings.TrimSpace(config["collection"])
			return config, nil
		},
	}
}

func validateQdrantStore(store domain.VectorStore) error {
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return err
	}
	if !collectionPattern.MatchString(store.Config["collection"]) {
		return saas.Invalid("Qdrant 集合前缀只允许1–100位字母、数字、下划线和横线")
	}
	return nil
}
