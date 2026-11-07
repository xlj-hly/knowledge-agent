package knowledge

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/httpclient"
)

//go:embed assets/tencent-vectordb.svg
var tencentVectorLogo []byte

// TencentVectorDBEntry 使用官方 SDK v1.8.4 对应的 HTTP 文档协议，不依赖其 gRPC 运行时。
func TencentVectorDBEntry() Entry {
	return Entry{Definition: dto.VectorProvider{ID: "tencent-vectordb", Name: "Tencent VectorDB", Description: "腾讯云向量数据库 · HTTP API · COSINE", Logo: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(tencentVectorLogo), Protocol: "tencent-vectordb", Fields: []dto.VectorField{
		{Key: "endpoint", Label: "服务地址", Type: "text", Required: true, Immutable: true, Placeholder: "http://your-vectordb-endpoint"},
		{Key: "username", Label: "账号", Type: "text", Required: true, Immutable: true, Default: "root"},
		{Key: "api_key", Label: "API Key", Type: "password", Required: true, Secret: true},
		{Key: "database", Label: "数据库", Type: "text", Required: true, Immutable: true, Default: "agent", Help: "使用已有数据库，集合将按向量维度自动建立。"},
		{Key: "collection", Label: "集合前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge"},
	}}, Driver: &tencentVectorDriver{client: httpclient.New()}, Validate: validateTencentVectorStore, Normalize: normalizeVectorEndpoint}
}

func validateTencentVectorStore(store domain.VectorStore) error {
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return err
	}
	if !vectorIdentifier(store.Config["database"], 100) || !vectorIdentifier(store.Config["collection"], 100) {
		return saas.Invalid("腾讯向量库数据库与集合前缀须以字母开头，只含字母、数字、下划线和横线")
	}
	if store.Config["username"] == "" || store.Config["api_key"] == "" || strings.ContainsAny(store.Config["username"], "&=\r\n") {
		return saas.Invalid("请填写有效腾讯向量库账号与 API Key")
	}
	return nil
}

type tencentVectorDriver struct{ client *http.Client }

func (v *tencentVectorDriver) call(ctx context.Context, s domain.VectorStore, path string, input, output any) error {
	if err := validateTencentVectorStore(s); err != nil {
		return err
	}
	headers := map[string]string{"Authorization": "Bearer account=" + s.Config["username"] + "&api_key=" + s.Config["api_key"]}
	var response json.RawMessage
	if _, err := vectorJSON(ctx, v.client, s, http.MethodPost, path, headers, input, &response); err != nil {
		return err
	}
	var status struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal(response, &status) != nil || status.Code == nil {
		return errors.New("腾讯向量库返回缺少状态码")
	}
	if *status.Code != 0 {
		return fmt.Errorf("腾讯向量库操作失败（状态码 %d）", *status.Code)
	}
	if output != nil && json.Unmarshal(response, output) != nil {
		return errors.New("腾讯向量库操作结果格式无效")
	}
	return nil
}

type tencentCollection struct {
	Collection string `json:"collection"`
	Indexes    []struct {
		FieldName  string `json:"fieldName"`
		FieldType  string `json:"fieldType"`
		IndexType  string `json:"indexType"`
		Dimension  int    `json:"dimension"`
		MetricType string `json:"metricType"`
	} `json:"indexes"`
}

func (v *tencentVectorDriver) collections(ctx context.Context, s domain.VectorStore) ([]tencentCollection, error) {
	var out struct {
		Collections []tencentCollection `json:"collections"`
	}
	err := v.call(ctx, s, "/collection/list", map[string]string{"database": s.Config["database"]}, &out)
	return out.Collections, err
}
func (v *tencentVectorDriver) Test(ctx context.Context, s domain.VectorStore) error {
	_, err := v.collections(ctx, s)
	return err
}
func tencentCollectionName(s domain.VectorStore, dim int) string {
	return s.Config["collection"] + "_" + strconv.Itoa(dim)
}

func validateTencentCollection(collection tencentCollection, dim int) error {
	vectorOK := false
	keys := map[string]bool{}
	for _, index := range collection.Indexes {
		if index.FieldName == "vector" && index.FieldType == "vector" && index.Dimension == dim && strings.EqualFold(index.MetricType, "COSINE") {
			vectorOK = true
		}
		if index.FieldType == "string" && (index.IndexType == "filter" || index.IndexType == "primaryKey") {
			keys[index.FieldName] = true
		}
	}
	if !vectorOK {
		return errors.New("腾讯向量库集合维度或 COSINE 度量与本应用不一致")
	}
	for _, key := range []string{"id", "tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"} {
		if !keys[key] {
			return errors.New("腾讯向量库集合缺少必要过滤索引")
		}
	}
	return nil
}

func (v *tencentVectorDriver) ensure(ctx context.Context, s domain.VectorStore, dim int) error {
	collections, err := v.collections(ctx, s)
	if err != nil {
		return err
	}
	name := tencentCollectionName(s, dim)
	for _, item := range collections {
		if item.Collection == name {
			return validateTencentCollection(item, dim)
		}
	}
	indexes := []map[string]any{{"fieldName": "id", "fieldType": "string", "indexType": "primaryKey"}, {"fieldName": "vector", "fieldType": "vector", "indexType": "HNSW", "dimension": dim, "metricType": "COSINE", "params": map[string]int{"M": 16, "efConstruction": 200}}}
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"} {
		indexes = append(indexes, map[string]any{"fieldName": key, "fieldType": "string", "indexType": "filter"})
	}
	input := map[string]any{"database": s.Config["database"], "collection": name, "replicaNum": 1, "shardNum": 1, "indexes": indexes}
	if err := v.call(ctx, s, "/collection/create", input, nil); err != nil {
		var out struct {
			Collection tencentCollection `json:"collection"`
		}
		if check := v.call(ctx, s, "/collection/describe", map[string]string{"database": s.Config["database"], "collection": name}, &out); check != nil {
			return err
		}
		return validateTencentCollection(out.Collection, dim)
	}
	return nil
}

func tencentFilter(t, b, d, a string) string {
	scope := vectorScope(t, b, d, a)
	keys := make([]string, 0, len(scope))
	for key := range scope {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, _ := json.Marshal(scope[key])
		parts = append(parts, key+" = "+string(value))
	}
	return strings.Join(parts, " and ")
}

func (v *tencentVectorDriver) ReplaceDocument(ctx context.Context, s domain.VectorStore, t, b, d, a string, points []port.VectorPoint) error {
	if err := vectorRequiredScope(s, t, b, d, a, true); err != nil {
		return err
	}
	dim, err := vectorPoints(points)
	if err != nil || dim == 0 {
		return err
	}
	if err := v.ensure(ctx, s, dim); err != nil {
		return err
	}
	batchSize := vectorBatchSize(dim)
	for start := 0; start < len(points); start += batchSize {
		rows := make([]map[string]any, 0, min(batchSize, len(points)-start))
		for _, point := range points[start:min(start+batchSize, len(points))] {
			row := map[string]any{"id": vectorUUID(t, b, d, a, point.ID), "vector": point.Vector, "chunk_id": point.ID}
			for key, value := range vectorScope(t, b, d, a) {
				row[key] = value
			}
			rows = append(rows, row)
		}
		var result struct {
			Count *int `json:"affectedCount"`
		}
		if err := v.call(ctx, s, "/document/upsert", map[string]any{"database": s.Config["database"], "collection": tencentCollectionName(s, dim), "buildIndex": true, "documents": rows}, &result); err != nil {
			return err
		}
		if result.Count == nil || *result.Count != len(rows) {
			return errors.New("Tencent VectorDB 未确认全部向量写入")
		}
	}
	return nil
}

func (v *tencentVectorDriver) DeleteDocument(ctx context.Context, s domain.VectorStore, t, b, d string) error {
	return v.delete(ctx, s, t, b, d, "")
}
func (v *tencentVectorDriver) DeleteAttempt(ctx context.Context, s domain.VectorStore, t, b, d, a string) error {
	if a == "" {
		return errors.New("清理索引必须指定处理代")
	}
	return v.delete(ctx, s, t, b, d, a)
}
func (v *tencentVectorDriver) delete(ctx context.Context, s domain.VectorStore, t, b, d, a string) error {
	if err := vectorRequiredScope(s, t, b, d, a, a != ""); err != nil {
		return err
	}
	collections, err := v.collections(ctx, s)
	if err != nil {
		return err
	}
	for _, item := range collections {
		dim, err := strconv.Atoi(strings.TrimPrefix(item.Collection, s.Config["collection"]+"_"))
		if err != nil || dim < 1 || dim > 65536 || item.Collection != tencentCollectionName(s, dim) {
			continue
		}
		if err := v.call(ctx, s, "/document/delete", map[string]any{"database": s.Config["database"], "collection": item.Collection, "query": map[string]any{"filter": tencentFilter(t, b, d, a)}}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (v *tencentVectorDriver) Search(ctx context.Context, s domain.VectorStore, t, b string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(t, b, query, limit); err != nil {
		return nil, err
	}
	if err := vectorRequiredScope(s, t, b, "search", "", false); err != nil {
		return nil, err
	}
	collections, err := v.collections(ctx, s)
	if err != nil {
		return nil, err
	}
	name := tencentCollectionName(s, len(query))
	exists := false
	for _, item := range collections {
		if item.Collection == name {
			if err := validateTencentCollection(item, len(query)); err != nil {
				return nil, err
			}
			exists = true
		}
	}
	if !exists {
		return []port.VectorHit{}, nil
	}
	var out struct {
		Documents [][]struct {
			Chunk    string  `json:"chunk_id"`
			Tenant   string  `json:"tenant_id"`
			Base     string  `json:"knowledge_base_id"`
			Document string  `json:"document_id"`
			Attempt  string  `json:"attempt"`
			Score    float64 `json:"score"`
		} `json:"documents"`
	}
	input := map[string]any{"database": s.Config["database"], "collection": name, "readConsistency": "strongConsistency", "search": map[string]any{"vectors": [][]float32{query}, "limit": limit, "filter": tencentFilter(t, b, "", ""), "outputFields": []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"}, "retrieveVector": false, "params": map[string]int{"ef": max(64, limit)}}}
	if err := v.call(ctx, s, "/document/search", input, &out); err != nil {
		return nil, err
	}
	if len(out.Documents) != 1 || len(out.Documents[0]) > limit {
		return nil, errors.New("腾讯向量库检索结果数量无效")
	}
	hits := []port.VectorHit{}
	for _, item := range out.Documents[0] {
		metadata := vectorScope(item.Tenant, item.Base, item.Document, item.Attempt)
		metadata["chunk_id"] = item.Chunk
		hit, err := vectorScopedHit(metadata, t, b, item.Chunk, item.Score)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

var _ port.Vectors = (*tencentVectorDriver)(nil)
