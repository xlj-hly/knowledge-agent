package knowledge

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/httpclient"
)

//go:embed assets/milvus.png
var milvusLogo []byte

//go:embed assets/zilliz.png
var zillizLogo []byte

// MilvusEntries 为 Milvus 和 Zilliz Cloud 共用显式的 REST v2 驱动。
func MilvusEntries() []Entry {
	driver := &milvusDriver{client: httpclient.New()}
	result := make([]Entry, 0, 2)
	for _, item := range []struct {
		id, name, endpoint string
		logo               []byte
	}{
		{"milvus", "Milvus", "http://host.docker.internal:19530", milvusLogo},
		{"zilliz", "Zilliz Cloud", "", zillizLogo},
	} {
		result = append(result, Entry{Definition: dto.VectorProvider{ID: item.id, Name: item.name, Description: "Milvus REST v2 · COSINE · 按维度隔离集合", Logo: "data:image/png;base64," + base64.StdEncoding.EncodeToString(item.logo), Protocol: "milvus", Fields: []dto.VectorField{
			{Key: "endpoint", Label: "服务地址", Type: "text", Required: true, Immutable: true, Default: item.endpoint, Placeholder: "https://your-cluster.api.gcp-us-west1.zillizcloud.com"},
			{Key: "token", Label: "Token", Type: "password", Secret: true, Help: "Milvus 可填写 username:password；Zilliz 填写 API Key。"},
			{Key: "database", Label: "数据库", Type: "text", Required: true, Immutable: true, Default: "default"},
			{Key: "collection", Label: "集合前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge"},
		}}, Driver: driver, Validate: validateMilvusStore, Normalize: normalizeVectorEndpoint})
	}
	return result
}

var milvusName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,100}$`)

func validateMilvusStore(store domain.VectorStore) error {
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return err
	}
	if !milvusName.MatchString(store.Config["database"]) || !milvusName.MatchString(store.Config["collection"]) {
		return saas.Invalid("Milvus 数据库和集合前缀须为字母、数字与下划线，且不能以数字开头")
	}
	return nil
}

type milvusDriver struct{ client *http.Client }

func (v *milvusDriver) call(ctx context.Context, store domain.VectorStore, operation string, input map[string]any, output any) error {
	if err := validateMilvusStore(store); err != nil {
		return err
	}
	input["dbName"] = store.Config["database"]
	headers := map[string]string{}
	if token := store.Config["token"]; token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	var response struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if _, err := vectorJSON(ctx, v.client, store, http.MethodPost, "/v2/vectordb/"+operation, headers, input, &response); err != nil {
		return err
	}
	if response.Code == nil || *response.Code != 0 {
		if response.Code == nil {
			return errors.New("Milvus 返回缺少状态码")
		}
		return fmt.Errorf("Milvus 操作失败（状态码 %d）", *response.Code)
	}
	if output != nil {
		if len(response.Data) == 0 || string(response.Data) == "null" || json.Unmarshal(response.Data, output) != nil {
			return errors.New("Milvus 返回无效操作结果")
		}
	}
	return nil
}

func (v *milvusDriver) collections(ctx context.Context, store domain.VectorStore) ([]string, error) {
	var names []string
	err := v.call(ctx, store, "collections/list", map[string]any{}, &names)
	return names, err
}

func (v *milvusDriver) Test(ctx context.Context, store domain.VectorStore) error {
	_, err := v.collections(ctx, store)
	return err
}

func milvusCollection(store domain.VectorStore, dimension int) string {
	return store.Config["collection"] + "_" + strconv.Itoa(dimension)
}

func (v *milvusDriver) ensure(ctx context.Context, store domain.VectorStore, dimension int) error {
	name := milvusCollection(store, dimension)
	var has struct {
		Has bool `json:"has"`
	}
	if err := v.call(ctx, store, "collections/has", map[string]any{"collectionName": name}, &has); err != nil {
		return err
	}
	if !has.Has {
		fields := []map[string]any{
			{"fieldName": "id", "dataType": "VarChar", "isPrimary": true, "elementTypeParams": map[string]string{"max_length": "64"}},
			{"fieldName": "vector", "dataType": "FloatVector", "elementTypeParams": map[string]string{"dim": strconv.Itoa(dimension)}},
		}
		input := map[string]any{"collectionName": name, "schema": map[string]any{"autoID": false, "enableDynamicField": true, "fields": fields}, "indexParams": []any{map[string]any{"fieldName": "vector", "indexName": "vector_index", "metricType": "COSINE", "params": map[string]string{"index_type": "AUTOINDEX"}}}}
		if err := v.call(ctx, store, "collections/create", input, nil); err != nil {
			// 并发创建只在重新读取并确认既定 schema 后才能视作成功。
			if check := v.describe(ctx, store, name, dimension); check != nil {
				return err
			}
		}
	}
	if err := v.describe(ctx, store, name, dimension); err != nil {
		return err
	}
	return v.call(ctx, store, "collections/load", map[string]any{"collectionName": name}, nil)
}

func (v *milvusDriver) describe(ctx context.Context, store domain.VectorStore, name string, dimension int) error {
	var out struct {
		Fields []struct {
			Name    string                        `json:"name"`
			Type    string                        `json:"type"`
			Primary bool                          `json:"primaryKey"`
			Params  []struct{ Key, Value string } `json:"params"`
		} `json:"fields"`
		Indexes []struct {
			FieldName  string `json:"fieldName"`
			MetricType string `json:"metricType"`
		} `json:"indexes"`
		Dynamic bool `json:"enableDynamicField"`
	}
	if err := v.call(ctx, store, "collections/describe", map[string]any{"collectionName": name}, &out); err != nil {
		return err
	}
	idOK, vectorOK, cosine := false, false, false
	for _, field := range out.Fields {
		if field.Name == "id" && field.Type == "VarChar" && field.Primary {
			idOK = true
		}
		if field.Name == "vector" && field.Type == "FloatVector" {
			for _, param := range field.Params {
				if param.Key == "dim" && param.Value == strconv.Itoa(dimension) {
					vectorOK = true
				}
			}
		}
	}
	for _, index := range out.Indexes {
		if index.FieldName == "vector" && strings.EqualFold(index.MetricType, "COSINE") {
			cosine = true
		}
	}
	if !idOK || !vectorOK || !cosine || !out.Dynamic {
		return errors.New("Milvus 集合结构、维度或 COSINE 度量与本应用不一致")
	}
	return nil
}

func milvusFilter(tenant, base, document, attempt string) string {
	metadata := vectorScope(tenant, base, document, attempt)
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, _ := json.Marshal(metadata[key])
		parts = append(parts, key+" == "+string(value))
	}
	return strings.Join(parts, " and ")
}

func (v *milvusDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, true); err != nil {
		return err
	}
	dim, err := vectorPoints(points)
	if err != nil || dim == 0 {
		return err
	}
	if err := v.ensure(ctx, store, dim); err != nil {
		return err
	}
	batchSize := vectorBatchSize(dim)
	for start := 0; start < len(points); start += batchSize {
		rows := make([]map[string]any, 0, min(batchSize, len(points)-start))
		for _, point := range points[start:min(start+batchSize, len(points))] {
			row := map[string]any{"id": vectorUUID(tenant, base, document, attempt, point.ID), "vector": point.Vector, "chunk_id": point.ID}
			for key, value := range vectorScope(tenant, base, document, attempt) {
				row[key] = value
			}
			rows = append(rows, row)
		}
		var result struct {
			Count *int `json:"upsertCount"`
		}
		if err := v.call(ctx, store, "entities/upsert", map[string]any{"collectionName": milvusCollection(store, dim), "data": rows}, &result); err != nil {
			return err
		}
		if result.Count == nil || *result.Count != len(rows) {
			return errors.New("Milvus 未确认全部向量写入")
		}
	}
	return nil
}

func (v *milvusDriver) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	return v.delete(ctx, store, tenant, base, document, "")
}
func (v *milvusDriver) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	if attempt == "" {
		return errors.New("清理索引必须指定处理代")
	}
	return v.delete(ctx, store, tenant, base, document, attempt)
}
func (v *milvusDriver) delete(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, attempt != ""); err != nil {
		return err
	}
	names, err := v.collections(ctx, store)
	if err != nil {
		return err
	}
	for _, name := range names {
		suffix := strings.TrimPrefix(name, store.Config["collection"]+"_")
		dim, err := strconv.Atoi(suffix)
		if err != nil || dim < 1 || name != milvusCollection(store, dim) {
			continue
		}
		if err := v.call(ctx, store, "entities/delete", map[string]any{"collectionName": name, "filter": milvusFilter(tenant, base, document, attempt)}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (v *milvusDriver) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(tenant, base, query, limit); err != nil {
		return nil, err
	}
	if err := vectorRequiredScope(store, tenant, base, "search", "", false); err != nil {
		return nil, err
	}
	name := milvusCollection(store, len(query))
	names, err := v.collections(ctx, store)
	if err != nil {
		return nil, err
	}
	found := false
	for _, item := range names {
		if item == name {
			found = true
		}
	}
	if !found {
		return []port.VectorHit{}, nil
	}
	if err := v.describe(ctx, store, name, len(query)); err != nil {
		return nil, err
	}
	var rows []struct {
		ChunkID  string  `json:"chunk_id"`
		Tenant   string  `json:"tenant_id"`
		Base     string  `json:"knowledge_base_id"`
		Document string  `json:"document_id"`
		Attempt  string  `json:"attempt"`
		Distance float64 `json:"distance"`
	}
	input := map[string]any{"collectionName": name, "data": [][]float32{query}, "annsField": "vector", "filter": milvusFilter(tenant, base, "", ""), "limit": limit, "outputFields": []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"}, "consistencyLevel": "Strong"}
	if err := v.call(ctx, store, "entities/search", input, &rows); err != nil {
		return nil, err
	}
	if len(rows) > limit {
		return nil, errors.New("Milvus 返回结果超过请求数量")
	}
	hits := make([]port.VectorHit, 0, len(rows))
	for _, row := range rows {
		metadata := vectorScope(row.Tenant, row.Base, row.Document, row.Attempt)
		metadata["chunk_id"] = row.ChunkID
		hit, err := vectorScopedHit(metadata, tenant, base, row.ChunkID, row.Distance)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

var _ port.Vectors = (*milvusDriver)(nil)
