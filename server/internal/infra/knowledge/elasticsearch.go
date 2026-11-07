package knowledge

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

//go:embed assets/elasticsearch.svg
var elasticsearchLogo []byte

//go:embed assets/opensearch.svg
var opensearchLogo []byte

// ElasticsearchEntries 注册 Elasticsearch 8 和带 k-NN 插件的 OpenSearch。
func ElasticsearchEntries() []Entry {
	result := make([]Entry, 0, 2)
	for _, item := range []struct {
		id, name string
		logo     []byte
	}{{"elasticsearch", "Elasticsearch", elasticsearchLogo}, {"opensearch", "OpenSearch", opensearchLogo}} {
		result = append(result, Entry{Definition: dto.VectorProvider{ID: item.id, Name: item.name, Description: "REST API · 独立维度索引 · 精确 COSINE 检索", Logo: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(item.logo), Protocol: item.id, Fields: []dto.VectorField{
			{Key: "endpoint", Label: "服务地址", Type: "text", Required: true, Immutable: true, Default: "http://host.docker.internal:9200"},
			{Key: "username", Label: "用户名", Type: "text", Immutable: true},
			{Key: "password", Label: "密码", Type: "password", Secret: true},
			{Key: "api_key", Label: "API Key", Type: "password", Secret: true, Help: "与用户名密码二选一；填写已编码的 API Key。"},
			{Key: "index", Label: "索引前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge"},
		}}, Driver: &elasticsearchDriver{client: httpclient.New(), openSearch: item.id == "opensearch"}, Validate: validateElasticsearchStore, Normalize: normalizeVectorEndpoint})
	}
	return result
}

var elasticIndex = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,99}$`)

func validateElasticsearchStore(store domain.VectorStore) error {
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return err
	}
	if !elasticIndex.MatchString(store.Config["index"]) {
		return saas.Invalid("索引前缀只能使用小写字母、数字、下划线和横线，且以字母开头")
	}
	if store.Config["api_key"] != "" && (store.Config["username"] != "" || store.Config["password"] != "") {
		return saas.Invalid("API Key 与用户名密码不能同时填写")
	}
	if store.Config["password"] != "" && store.Config["username"] == "" {
		return saas.Invalid("使用密码时必须填写用户名")
	}
	return nil
}

type elasticsearchDriver struct {
	client     *http.Client
	openSearch bool
}

func elasticHeaders(store domain.VectorStore) map[string]string {
	headers := map[string]string{}
	if key := store.Config["api_key"]; key != "" {
		headers["Authorization"] = "ApiKey " + key
	} else if store.Config["username"] != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(store.Config["username"]+":"+store.Config["password"]))
	}
	return headers
}

func (v *elasticsearchDriver) call(ctx context.Context, store domain.VectorStore, method, path string, input, output any) (int, error) {
	if err := validateElasticsearchStore(store); err != nil {
		return 0, err
	}
	return vectorJSON(ctx, v.client, store, method, path, elasticHeaders(store), input, output)
}

func (v *elasticsearchDriver) Test(ctx context.Context, store domain.VectorStore) error {
	var info struct {
		Version struct{ Number, Distribution string } `json:"version"`
		Tagline string                                `json:"tagline"`
	}
	if _, err := v.call(ctx, store, http.MethodGet, "/", nil, &info); err != nil {
		return err
	}
	if v.openSearch {
		if info.Version.Distribution != "opensearch" {
			return errors.New("目标服务不是 OpenSearch")
		}
	} else if !strings.HasPrefix(info.Version.Number, "8.") && !strings.HasPrefix(info.Version.Number, "9.") {
		return errors.New("当前 Elasticsearch 驱动要求 8.x 或 9.x 服务")
	}
	return nil
}

func elasticIndexName(store domain.VectorStore, dim int) string {
	return store.Config["index"] + "_" + strconv.Itoa(dim)
}

func (v *elasticsearchDriver) mapping(ctx context.Context, store domain.VectorStore, name string, dim int) (bool, error) {
	var mapping map[string]struct {
		Mappings struct {
			Properties map[string]struct {
				Type       string `json:"type"`
				Dims       int    `json:"dims"`
				Dimension  int    `json:"dimension"`
				Similarity string `json:"similarity"`
				Method     struct {
					SpaceType string `json:"space_type"`
				} `json:"method"`
			} `json:"properties"`
		} `json:"mappings"`
	}
	status, err := v.call(ctx, store, http.MethodGet, "/"+name+"/_mapping", nil, &mapping)
	if status == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	index, ok := mapping[name]
	if !ok {
		return false, errors.New("索引映射响应缺少目标索引")
	}
	field, ok := index.Mappings.Properties["vector"]
	if !ok {
		return false, errors.New("索引缺少向量字段")
	}
	valid := field.Type == "dense_vector" && field.Dims == dim && field.Similarity == "cosine"
	if v.openSearch {
		valid = field.Type == "knn_vector" && field.Dimension == dim && field.Method.SpaceType == "cosinesimil"
	}
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"} {
		if index.Mappings.Properties[key].Type != "keyword" {
			valid = false
		}
	}
	if !valid {
		return false, errors.New("索引结构、维度或 COSINE 度量与本应用不一致")
	}
	return true, nil
}

func (v *elasticsearchDriver) ensure(ctx context.Context, store domain.VectorStore, dim int) error {
	name := elasticIndexName(store, dim)
	exists, err := v.mapping(ctx, store, name, dim)
	if err != nil || exists {
		return err
	}
	properties := map[string]any{}
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"} {
		properties[key] = map[string]string{"type": "keyword"}
	}
	vector := map[string]any{"type": "dense_vector", "dims": dim, "similarity": "cosine"}
	input := map[string]any{"mappings": map[string]any{"dynamic": "strict", "properties": properties}}
	if v.openSearch {
		vector = map[string]any{"type": "knn_vector", "dimension": dim, "method": map[string]any{"name": "hnsw", "space_type": "cosinesimil", "engine": "lucene", "parameters": map[string]int{"m": 16, "ef_construction": 128}}}
		input["settings"] = map[string]any{"index": map[string]any{"knn": true}}
	}
	properties["vector"] = vector
	if _, err := v.call(ctx, store, http.MethodPut, "/"+name, input, nil); err != nil {
		if ok, check := v.mapping(ctx, store, name, dim); check != nil || !ok {
			return err
		}
	}
	_, err = v.mapping(ctx, store, name, dim)
	return err
}

func (v *elasticsearchDriver) raw(ctx context.Context, store domain.VectorStore, path string, body []byte, output any) error {
	address, err := httpclient.Endpoint(store.Config["endpoint"], "")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("无法创建索引批量请求")
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	for key, value := range elasticHeaders(store) {
		req.Header.Set(key, value)
	}
	response, err := v.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("索引批量服务连接失败")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil {
		return errors.New("读取索引批量结果失败")
	}
	if len(data) > 8<<20 {
		return errors.New("索引批量响应超过大小限制")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("索引批量服务返回 HTTP %d", response.StatusCode)
	}
	if json.Unmarshal(data, output) != nil {
		return errors.New("索引批量结果格式无效")
	}
	return nil
}

func (v *elasticsearchDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
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
		var body bytes.Buffer
		encoder := json.NewEncoder(&body)
		for _, point := range points[start:min(start+batchSize, len(points))] {
			if err := encoder.Encode(map[string]any{"index": map[string]string{"_id": vectorUUID(tenant, base, document, attempt, point.ID)}}); err != nil {
				return err
			}
			row := map[string]any{"vector": point.Vector, "chunk_id": point.ID}
			for key, value := range vectorScope(tenant, base, document, attempt) {
				row[key] = value
			}
			if err := encoder.Encode(row); err != nil {
				return err
			}
		}
		var result struct {
			Errors bool `json:"errors"`
			Items  []map[string]struct {
				Status int `json:"status"`
			} `json:"items"`
		}
		if err := v.raw(ctx, store, "/"+elasticIndexName(store, dim)+"/_bulk?refresh=wait_for", body.Bytes(), &result); err != nil {
			return err
		}
		if result.Errors || len(result.Items) != min(batchSize, len(points)-start) {
			return errors.New("索引批量写入未全部成功")
		}
		for _, item := range result.Items {
			operation, ok := item["index"]
			if !ok || operation.Status < 200 || operation.Status >= 300 {
				return errors.New("索引批量写入包含失败记录")
			}
		}
	}
	return nil
}

func elasticFilter(tenant, base, document, attempt string) map[string]any {
	terms := []any{}
	for key, value := range vectorScope(tenant, base, document, attempt) {
		terms = append(terms, map[string]any{"term": map[string]string{key: value}})
	}
	return map[string]any{"bool": map[string]any{"filter": terms}}
}

func (v *elasticsearchDriver) DeleteDocument(ctx context.Context, s domain.VectorStore, t, b, d string) error {
	return v.delete(ctx, s, t, b, d, "")
}
func (v *elasticsearchDriver) DeleteAttempt(ctx context.Context, s domain.VectorStore, t, b, d, a string) error {
	if a == "" {
		return errors.New("清理索引必须指定处理代")
	}
	return v.delete(ctx, s, t, b, d, a)
}
func (v *elasticsearchDriver) delete(ctx context.Context, s domain.VectorStore, t, b, d, a string) error {
	if err := vectorRequiredScope(s, t, b, d, a, a != ""); err != nil {
		return err
	}
	var indexes []struct {
		Index string `json:"index"`
	}
	status, err := v.call(ctx, s, http.MethodGet, "/_cat/indices/"+s.Config["index"]+"_*?format=json&h=index", nil, &indexes)
	if status == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	// 相同前缀可能还有人工维护的索引，只清理本驱动拥有的数字维度后缀。
	for _, index := range indexes {
		dimension, err := strconv.Atoi(strings.TrimPrefix(index.Index, s.Config["index"]+"_"))
		if err != nil || dimension < 1 || dimension > 65536 || index.Index != elasticIndexName(s, dimension) {
			continue
		}
		var out struct {
			Failures []json.RawMessage `json:"failures"`
			TimedOut bool              `json:"timed_out"`
		}
		status, err := v.call(ctx, s, http.MethodPost, "/"+index.Index+"/_delete_by_query?refresh=true&conflicts=proceed", map[string]any{"query": elasticFilter(t, b, d, a)}, &out)
		if status == http.StatusNotFound {
			continue
		}
		if err != nil {
			return err
		}
		if out.TimedOut || len(out.Failures) > 0 {
			return errors.New("索引删除未完成")
		}
	}
	return nil
}

func (v *elasticsearchDriver) Search(ctx context.Context, s domain.VectorStore, t, b string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(t, b, query, limit); err != nil {
		return nil, err
	}
	if err := vectorRequiredScope(s, t, b, "search", "", false); err != nil {
		return nil, err
	}
	exists, err := v.mapping(ctx, s, elasticIndexName(s, len(query)), len(query))
	if err != nil {
		return nil, err
	}
	if !exists {
		return []port.VectorHit{}, nil
	}
	// 显式计算 cosine+1，避免 OpenSearch 2.19 前后 knn_score 的版本相关归一差异。
	source := "cosineSimilarity(params.query, 'vector') + 1.0"
	if v.openSearch {
		source = "cosineSimilarity(params.query, doc['vector']) + 1.0"
	}
	script := map[string]any{"source": source, "lang": "painless", "params": map[string]any{"query": query}}
	var out struct {
		TimedOut bool `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits *struct {
			Hits []struct {
				Score  float64           `json:"_score"`
				Source map[string]string `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	input := map[string]any{"size": limit, "_source": []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"}, "query": map[string]any{"script_score": map[string]any{"query": elasticFilter(t, b, "", ""), "script": script}}}
	if _, err := v.call(ctx, s, http.MethodPost, "/"+elasticIndexName(s, len(query))+"/_search", input, &out); err != nil {
		return nil, err
	}
	if out.Hits == nil || out.Hits.Hits == nil || out.TimedOut || out.Shards.Failed > 0 || len(out.Hits.Hits) > limit {
		return nil, errors.New("索引检索未完整成功")
	}
	hits := []port.VectorHit{}
	for _, row := range out.Hits.Hits {
		hit, err := vectorScopedHit(row.Source, t, b, row.Source["chunk_id"], row.Score-1)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

var _ port.Vectors = (*elasticsearchDriver)(nil)
