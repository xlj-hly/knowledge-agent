package knowledge

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/httpclient"
)

//go:embed assets/weaviate.png
var weaviateLogo []byte

// WeaviateEntry 使用 REST schema、batch 和 GraphQL；自建及 Cloud 共用同一协议。
func WeaviateEntry() Entry {
	return Entry{Definition: dto.VectorProvider{ID: "weaviate", Name: "Weaviate", Description: "Weaviate REST / GraphQL · 自建与 Cloud · 自带向量", Logo: "data:image/png;base64," + base64.StdEncoding.EncodeToString(weaviateLogo), Protocol: "weaviate", Fields: []dto.VectorField{
		{Key: "endpoint", Label: "服务地址", Type: "text", Required: true, Immutable: true, Default: "http://host.docker.internal:8080"},
		{Key: "collection", Label: "Collection 前缀", Type: "text", Required: true, Immutable: true, Default: "AgentKnowledge", Help: "首字母须大写，只能包含字母、数字和下划线。"},
		{Key: "api_key", Label: "API Key", Type: "password", Secret: true, Help: "启用 API Key 鉴权时填写。"},
	}}, Driver: &weaviateDriver{client: httpclient.New()}, Validate: validateWeaviateStore, Normalize: normalizeVectorEndpoint}
}

type weaviateConfig struct{ collection, key string }
type weaviateDriver struct{ client *http.Client }

func decodeWeaviate(store domain.VectorStore) (weaviateConfig, error) {
	c := weaviateConfig{collection: store.Config["collection"], key: store.Config["api_key"]}
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return c, err
	}
	if !regexp.MustCompile(`^[A-Z][A-Za-z0-9_]{0,99}$`).MatchString(c.collection) {
		return c, saas.Invalid("Weaviate Collection 前缀须以大写字母开头且只含字母、数字、下划线")
	}
	return c, nil
}

func validateWeaviateStore(store domain.VectorStore) error {
	_, err := decodeWeaviate(store)
	return err
}

func (v *weaviateDriver) request(ctx context.Context, store domain.VectorStore, method, path string, input, output any) (int, error) {
	c, err := decodeWeaviate(store)
	if err != nil {
		return 0, err
	}
	headers := map[string]string{}
	if c.key != "" {
		headers["Authorization"] = "Bearer " + c.key
	}
	return vectorJSON(ctx, v.client, store, method, "/v1"+path, headers, input, output)
}

type weaviateClass struct {
	Class       string `json:"class"`
	Description string `json:"description"`
	Vectorizer  string `json:"vectorizer"`
	IndexConfig struct {
		Distance string `json:"distance"`
	} `json:"vectorIndexConfig"`
}

func (v *weaviateDriver) classes(ctx context.Context, store domain.VectorStore) ([]weaviateClass, error) {
	var result struct {
		Classes *[]weaviateClass `json:"classes"`
	}
	if _, err := v.request(ctx, store, http.MethodGet, "/schema", nil, &result); err != nil {
		return nil, err
	}
	if result.Classes == nil {
		return nil, errors.New("Weaviate 返回的 schema 无效")
	}
	return *result.Classes, nil
}

func (v *weaviateDriver) Test(ctx context.Context, store domain.VectorStore) error {
	_, err := v.classes(ctx, store)
	return err
}

func (v *weaviateDriver) class(ctx context.Context, store domain.VectorStore, dimension int, create bool) (string, bool, error) {
	c, err := decodeWeaviate(store)
	if err != nil {
		return "", false, err
	}
	name := c.collection + "_" + strconv.Itoa(dimension)
	var class weaviateClass
	status, err := v.request(ctx, store, http.MethodGet, "/schema/"+name, nil, &class)
	if status == http.StatusNotFound {
		if !create {
			return name, false, nil
		}
		properties := make([]map[string]any, 0, 5)
		for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"} {
			properties = append(properties, map[string]any{"name": key, "dataType": []string{"text"}, "tokenization": "field", "indexFilterable": true})
		}
		input := map[string]any{"class": name, "description": "agent-vector-dimension:" + strconv.Itoa(dimension), "vectorizer": "none", "vectorIndexConfig": map[string]string{"distance": "cosine"}, "properties": properties}
		status, err = v.request(ctx, store, http.MethodPost, "/schema", input, nil)
		if err != nil && status != http.StatusUnprocessableEntity && status != http.StatusConflict {
			return "", false, err
		}
		// 并发创建只在重新读取并确认模式后接受，不吞掉模式冲突。
		_, err = v.request(ctx, store, http.MethodGet, "/schema/"+name, nil, &class)
	}
	if err != nil {
		return "", false, err
	}
	if class.Class != name || class.Vectorizer != "none" || class.IndexConfig.Distance != "cosine" || class.Description != "agent-vector-dimension:"+strconv.Itoa(dimension) {
		return "", false, errors.New("Weaviate Collection 的维度声明、向量化方式或距离度量不匹配")
	}
	return name, true, nil
}

func weaviateWhere(tenant, base, document, attempt string) map[string]any {
	scope := vectorScope(tenant, base, document, attempt)
	conditions := make([]map[string]any, 0, len(scope))
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt"} {
		if value, exists := scope[key]; exists {
			conditions = append(conditions, map[string]any{"path": []string{key}, "operator": "Equal", "valueText": value})
		}
	}
	return map[string]any{"operator": "And", "operands": conditions}
}

func (v *weaviateDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
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
	class, _, err := v.class(ctx, store, dimension, true)
	if err != nil {
		return err
	}
	if err := v.DeleteAttempt(ctx, store, tenant, base, document, attempt); err != nil {
		return err
	}
	batchSize := vectorBatchSize(dimension)
	for start := 0; start < len(points); start += batchSize {
		batch := points[start:min(start+batchSize, len(points))]
		objects := make([]map[string]any, 0, len(batch))
		for _, point := range batch {
			properties := vectorScope(tenant, base, document, attempt)
			properties["chunk_id"] = point.ID
			objects = append(objects, map[string]any{"class": class, "id": vectorUUID(tenant, base, document, attempt, point.ID), "vector": point.Vector, "properties": properties})
		}
		var result []struct {
			Result struct {
				Status string          `json:"status"`
				Errors json.RawMessage `json:"errors"`
			} `json:"result"`
		}
		if _, err := v.request(ctx, store, http.MethodPost, "/batch/objects", map[string]any{"objects": objects}, &result); err != nil {
			return err
		}
		if len(result) != len(objects) {
			return errors.New("Weaviate 批量写入返回的记录数不匹配")
		}
		for _, row := range result {
			if row.Result.Status != "SUCCESS" || len(row.Result.Errors) > 0 && string(row.Result.Errors) != "null" {
				return errors.New("Weaviate 批量写入包含失败记录")
			}
		}
	}
	return nil
}

func (v *weaviateDriver) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	return v.delete(ctx, store, tenant, base, document, "", false)
}

func (v *weaviateDriver) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	return v.delete(ctx, store, tenant, base, document, attempt, true)
}

func (v *weaviateDriver) delete(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, exact bool) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, exact); err != nil {
		return err
	}
	c, err := decodeWeaviate(store)
	if err != nil {
		return err
	}
	classes, err := v.classes(ctx, store)
	if err != nil {
		return err
	}
	for _, class := range classes {
		dimension, err := strconv.Atoi(strings.TrimPrefix(class.Class, c.collection+"_"))
		if err != nil || dimension < 1 || dimension > 65536 || class.Class != c.collection+"_"+strconv.Itoa(dimension) {
			continue
		}
		for page := 0; ; page++ {
			var result struct {
				Results *struct {
					Matches, Successful, Failed int
				} `json:"results"`
			}
			status, err := v.request(ctx, store, http.MethodDelete, "/batch/objects", map[string]any{"match": map[string]any{"class": class.Class, "where": weaviateWhere(tenant, base, document, attempt)}, "output": "minimal", "dryRun": false}, &result)
			if status == http.StatusNotFound {
				break
			}
			if err != nil {
				return err
			}
			if result.Results == nil || result.Results.Failed != 0 {
				return errors.New("Weaviate 批量删除包含失败记录")
			}
			if result.Results.Matches <= result.Results.Successful {
				break
			}
			if result.Results.Successful == 0 || page >= 1000 {
				return errors.New("Weaviate 批量删除未完成")
			}
		}
	}
	return nil
}

func (v *weaviateDriver) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(tenant, base, query, limit); err != nil {
		return nil, err
	}
	class, exists, err := v.class(ctx, store, len(query), false)
	if err != nil || !exists {
		return []port.VectorHit{}, err
	}
	vector, _ := json.Marshal(query)
	tenantValue, _ := json.Marshal(tenant)
	baseValue, _ := json.Marshal(base)
	graph := "{Get{" + class + "(nearVector:{vector:" + string(vector) + "},limit:" + strconv.Itoa(limit) + ",where:{operator:And,operands:[{path:[\"tenant_id\"],operator:Equal,valueText:" + string(tenantValue) + "},{path:[\"knowledge_base_id\"],operator:Equal,valueText:" + string(baseValue) + "}]}){tenant_id knowledge_base_id document_id attempt chunk_id _additional{id distance}}}}"
	var result struct {
		Data struct {
			Get map[string][]struct {
				Tenant   string `json:"tenant_id"`
				Base     string `json:"knowledge_base_id"`
				Document string `json:"document_id"`
				Attempt  string `json:"attempt"`
				Chunk    string `json:"chunk_id"`
				Extra    struct {
					ID       string   `json:"id"`
					Distance *float64 `json:"distance"`
				} `json:"_additional"`
			} `json:"Get"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if _, err := v.request(ctx, store, http.MethodPost, "/graphql", map[string]string{"query": graph}, &result); err != nil {
		return nil, err
	}
	rows, exists := result.Data.Get[class]
	if len(result.Errors) > 0 || !exists || len(rows) > limit {
		return nil, errors.New("Weaviate GraphQL 检索失败或响应无效")
	}
	hits := make([]port.VectorHit, 0, len(rows))
	for _, row := range rows {
		if row.Extra.Distance == nil || row.Extra.ID != vectorUUID(tenant, base, row.Document, row.Attempt, row.Chunk) {
			return nil, errors.New("Weaviate 记录标识或距离无效")
		}
		metadata := vectorScope(row.Tenant, row.Base, row.Document, row.Attempt)
		metadata["chunk_id"] = row.Chunk
		hit, err := vectorScopedHit(metadata, tenant, base, row.Chunk, 1-*row.Extra.Distance)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
