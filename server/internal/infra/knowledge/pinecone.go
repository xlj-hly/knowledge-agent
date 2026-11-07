package knowledge

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/httpclient"
)

//go:embed assets/pinecone.png
var pineconeLogo []byte

// PineconeEntry 使用现有 cosine 索引的 data-plane host，不隐式创建付费索引。
func PineconeEntry() Entry {
	return Entry{Definition: dto.VectorProvider{ID: "pinecone", Name: "Pinecone", Description: "Pinecone dense cosine 索引 · 使用索引专属 Host", Logo: "data:image/png;base64," + base64.StdEncoding.EncodeToString(pineconeLogo), Protocol: "pinecone", Fields: []dto.VectorField{
		{Key: "endpoint", Label: "Index Host", Type: "text", Required: true, Immutable: true, Placeholder: "https://your-index.svc.region.pinecone.io", Help: "从 Pinecone 控制台复制索引的 Host；索引须已创建且距离度量为 cosine。"},
		{Key: "namespace", Label: "Namespace 前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge", Help: "系统在此前缀后添加企业与知识库的确定性标识。"},
		{Key: "api_key", Label: "API Key", Type: "password", Required: true, Secret: true},
	}}, Driver: &pineconeDriver{client: httpclient.New()}, Validate: validatePineconeStore, Normalize: normalizeVectorEndpoint}
}

type pineconeConfig struct{ namespace, key string }
type pineconeDriver struct{ client *http.Client }

func decodePinecone(store domain.VectorStore) (pineconeConfig, error) {
	c := pineconeConfig{namespace: store.Config["namespace"], key: store.Config["api_key"]}
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return c, err
	}
	if !vectorIdentifier(c.namespace, 100) || c.key == "" {
		return c, saas.Invalid("Pinecone 需要有效 Namespace 前缀和 API Key")
	}
	return c, nil
}

func validatePineconeStore(store domain.VectorStore) error {
	_, err := decodePinecone(store)
	return err
}

func (v *pineconeDriver) request(ctx context.Context, store domain.VectorStore, path string, input, output any) error {
	c, err := decodePinecone(store)
	if err != nil {
		return err
	}
	_, err = vectorJSON(ctx, v.client, store, http.MethodPost, path, map[string]string{"Api-Key": c.key, "X-Pinecone-API-Version": "2025-10"}, input, output)
	return err
}

func (v *pineconeDriver) dimension(ctx context.Context, store domain.VectorStore) (int, error) {
	var stats struct {
		Dimension int    `json:"dimension"`
		Metric    string `json:"metric"`
	}
	if err := v.request(ctx, store, "/describe_index_stats", map[string]any{}, &stats); err != nil {
		return 0, err
	}
	if stats.Dimension < 1 || stats.Dimension > 65536 || stats.Metric != "cosine" {
		return 0, errors.New("Pinecone 索引必须为有效维度的 dense cosine 索引")
	}
	return stats.Dimension, nil
}

func (v *pineconeDriver) Test(ctx context.Context, store domain.VectorStore) error {
	_, err := v.dimension(ctx, store)
	return err
}

func pineconeNamespace(store domain.VectorStore, tenant, base string) string {
	return store.Config["namespace"] + "_" + strings.ReplaceAll(vectorUUID(tenant, base, "", "", ""), "-", "")
}

func pineconeFilter(tenant, base, document, attempt string) map[string]any {
	clauses := make([]map[string]any, 0, 4)
	scope := vectorScope(tenant, base, document, attempt)
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt"} {
		if value, exists := scope[key]; exists {
			clauses = append(clauses, map[string]any{key: map[string]string{"$eq": value}})
		}
	}
	return map[string]any{"$and": clauses}
}

func (v *pineconeDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, true); err != nil {
		return err
	}
	dimension, err := vectorPoints(points)
	if err != nil {
		return err
	}
	if len(points) == 0 {
		return v.DeleteAttempt(ctx, store, tenant, base, document, attempt)
	}
	configured, err := v.dimension(ctx, store)
	if err != nil {
		return err
	}
	if configured != dimension {
		return errors.New("Pinecone 索引维度与 Embedding 模型不一致")
	}
	if err := v.DeleteAttempt(ctx, store, tenant, base, document, attempt); err != nil {
		return err
	}
	// Pinecone upsert 请求上限为 2 MB，保守预留 metadata 和 JSON 数字编码空间。
	batchSize := max(1, min(vectorBatchSize(dimension), 1_500_000/(dimension*16+4096)))
	for start := 0; start < len(points); start += batchSize {
		batch := points[start:min(start+batchSize, len(points))]
		rows := make([]map[string]any, 0, len(batch))
		for _, point := range batch {
			metadata := vectorScope(tenant, base, document, attempt)
			metadata["chunk_id"] = point.ID
			rows = append(rows, map[string]any{"id": vectorUUID(tenant, base, document, attempt, point.ID), "values": point.Vector, "metadata": metadata})
		}
		var result struct {
			Count *int `json:"upsertedCount"`
		}
		if err := v.request(ctx, store, "/vectors/upsert", map[string]any{"namespace": pineconeNamespace(store, tenant, base), "vectors": rows}, &result); err != nil {
			return err
		}
		if result.Count == nil || *result.Count != len(rows) {
			return errors.New("Pinecone 批量写入未全部完成")
		}
	}
	return nil
}

func (v *pineconeDriver) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	return v.delete(ctx, store, tenant, base, document, "", false)
}

func (v *pineconeDriver) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	return v.delete(ctx, store, tenant, base, document, attempt, true)
}

func (v *pineconeDriver) delete(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, exact bool) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, exact); err != nil {
		return err
	}
	return v.request(ctx, store, "/vectors/delete", map[string]any{"namespace": pineconeNamespace(store, tenant, base), "filter": pineconeFilter(tenant, base, document, attempt)}, nil)
}

func (v *pineconeDriver) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(tenant, base, query, limit); err != nil {
		return nil, err
	}
	dimension, err := v.dimension(ctx, store)
	if err != nil {
		return nil, err
	}
	if dimension != len(query) {
		return nil, errors.New("Pinecone 查询向量维度不匹配")
	}
	var result struct {
		Matches *[]struct {
			ID       string            `json:"id"`
			Score    *float64          `json:"score"`
			Metadata map[string]string `json:"metadata"`
		} `json:"matches"`
	}
	if err := v.request(ctx, store, "/query", map[string]any{"namespace": pineconeNamespace(store, tenant, base), "vector": query, "topK": limit, "filter": pineconeFilter(tenant, base, "", ""), "includeMetadata": true}, &result); err != nil {
		return nil, err
	}
	if result.Matches == nil || len(*result.Matches) > limit {
		return nil, errors.New("Pinecone 返回的检索数量无效")
	}
	hits := make([]port.VectorHit, 0, len(*result.Matches))
	for _, item := range *result.Matches {
		if item.Score == nil {
			return nil, errors.New("Pinecone 检索结果缺少相似度")
		}
		hit, err := vectorScopedHit(item.Metadata, tenant, base, item.Metadata["chunk_id"], *item.Score)
		if err != nil {
			return nil, err
		}
		if item.ID != vectorUUID(tenant, base, item.Metadata["document_id"], item.Metadata["attempt"], hit.ID) {
			return nil, errors.New("Pinecone 记录标识与归属不匹配")
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
