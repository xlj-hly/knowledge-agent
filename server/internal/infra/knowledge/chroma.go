package knowledge

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/httpclient"
)

//go:embed assets/chroma.png
var chromaLogo []byte

// ChromaEntries 将自建 Chroma 与 Chroma Cloud 的连接字段关联到同一个 v2 REST 驱动。
func ChromaEntries() []Entry {
	entries := make([]Entry, 0, 2)
	for _, cloud := range []bool{false, true} {
		id, name, endpoint, tenant, database := "chroma", "Chroma", "http://host.docker.internal:8000", "default_tenant", "default_database"
		if cloud {
			id, name, endpoint, tenant, database = "chromacloud", "Chroma Cloud", "https://api.trychroma.com", "", ""
		}
		entries = append(entries, Entry{Definition: dto.VectorProvider{ID: id, Name: name, Description: "Chroma v2 REST · 按向量维度建立 cosine 集合", Logo: "data:image/png;base64," + base64.StdEncoding.EncodeToString(chromaLogo), Protocol: "chroma", Fields: []dto.VectorField{
			{Key: "endpoint", Label: "服务地址", Type: "text", Required: true, Immutable: true, Default: endpoint},
			{Key: "tenant", Label: "Chroma Tenant", Type: "text", Required: true, Immutable: true, Default: tenant},
			{Key: "database", Label: "数据库", Type: "text", Required: true, Immutable: true, Default: database},
			{Key: "collection", Label: "集合前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge"},
			{Key: "api_key", Label: "API Key", Type: "password", Secret: true, Required: cloud, Help: "通过 X-Chroma-Token 发送；自建未启用鉴权时可留空。"},
		}}, Driver: &chromaDriver{client: httpclient.New(), cloud: cloud}, Validate: validateChromaStore, Normalize: normalizeVectorEndpoint})
	}
	return entries
}

type chromaConfig struct{ tenant, database, collection, key string }
type chromaDriver struct {
	client *http.Client
	cloud  bool
}

func decodeChroma(store domain.VectorStore) (chromaConfig, error) {
	c := chromaConfig{tenant: store.Config["tenant"], database: store.Config["database"], collection: store.Config["collection"], key: store.Config["api_key"]}
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return c, err
	}
	if !vectorIdentifier(c.collection, 48) || len(c.collection) < 3 || c.tenant == "" || c.database == "" || len(c.tenant) > 256 || len(c.database) > 256 || strings.ContainsAny(c.tenant+c.database, "/\\\x00") {
		return c, saas.Invalid("Chroma 的 Tenant、数据库或集合前缀无效")
	}
	if store.Provider == "chromacloud" && c.key == "" {
		return c, saas.Invalid("Chroma Cloud 需要 API Key")
	}
	return c, nil
}

func validateChromaStore(store domain.VectorStore) error { _, err := decodeChroma(store); return err }

func (v *chromaDriver) request(ctx context.Context, store domain.VectorStore, method, path string, input, output any) (int, error) {
	c, err := decodeChroma(store)
	if err != nil {
		return 0, err
	}
	headers := map[string]string{}
	if c.key != "" {
		headers["X-Chroma-Token"] = c.key
	}
	root := "/api/v2/tenants/" + url.PathEscape(c.tenant) + "/databases/" + url.PathEscape(c.database)
	return vectorJSON(ctx, v.client, store, method, root+path, headers, input, output)
}

type chromaCollection struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Dimension     *int           `json:"dimension"`
	Metadata      map[string]any `json:"metadata"`
	Configuration struct {
		HNSW  map[string]any `json:"hnsw"`
		SPANN map[string]any `json:"spann"`
	} `json:"configuration_json"`
}

func (v *chromaDriver) collections(ctx context.Context, store domain.VectorStore) ([]chromaCollection, error) {
	collections := []chromaCollection{}
	for offset := 0; offset < 100000; offset += 100 {
		var page []chromaCollection
		if _, err := v.request(ctx, store, http.MethodGet, "/collections?limit=100&offset="+strconv.Itoa(offset), nil, &page); err != nil {
			return nil, err
		}
		if page == nil {
			return nil, errors.New("Chroma 返回了无效的集合列表")
		}
		collections = append(collections, page...)
		if len(page) < 100 {
			return collections, nil
		}
	}
	return nil, errors.New("Chroma 集合数量超过本次扫描上限")
}

func (v *chromaDriver) Test(ctx context.Context, store domain.VectorStore) error {
	_, err := v.collections(ctx, store)
	return err
}

func (v *chromaDriver) collection(ctx context.Context, store domain.VectorStore, dimension int, create bool) (chromaCollection, bool, error) {
	c, err := decodeChroma(store)
	if err != nil {
		return chromaCollection{}, false, err
	}
	name := c.collection + "_" + strconv.Itoa(dimension)
	var collection chromaCollection
	status, err := v.request(ctx, store, http.MethodGet, "/collections/"+name, nil, &collection)
	if status == http.StatusNotFound {
		if !create {
			return collection, false, nil
		}
		index := "hnsw"
		if v.cloud {
			index = "spann"
		}
		_, err = v.request(ctx, store, http.MethodPost, "/collections", map[string]any{"name": name, "get_or_create": true, "metadata": map[string]any{"agent_dimension": dimension}, "configuration": map[string]any{index: map[string]string{"space": "cosine"}}}, &collection)
	}
	if err != nil {
		return collection, false, err
	}
	metric, _ := collection.Configuration.HNSW["space"].(string)
	if metric == "" {
		metric, _ = collection.Configuration.SPANN["space"].(string)
	}
	if metric == "" {
		metric, _ = collection.Metadata["hnsw:space"].(string)
	}
	if collection.ID == "" || collection.Name != name || collection.Dimension != nil && *collection.Dimension != dimension || metric != "cosine" {
		return collection, false, errors.New("Chroma 集合的标识、向量维度或距离度量不匹配")
	}
	return collection, true, nil
}

func chromaWhere(tenant, base, document, attempt string) map[string]any {
	conditions := make([]map[string]any, 0, 4)
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt"} {
		if value, exists := vectorScope(tenant, base, document, attempt)[key]; exists {
			conditions = append(conditions, map[string]any{key: map[string]string{"$eq": value}})
		}
	}
	return map[string]any{"$and": conditions}
}

func (v *chromaDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
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
	if v.cloud && dimension > 4096 {
		return errors.New("Chroma Cloud 向量维度不能超过 4096")
	}
	collection, _, err := v.collection(ctx, store, dimension, true)
	if err != nil {
		return err
	}
	if err := v.DeleteAttempt(ctx, store, tenant, base, document, attempt); err != nil {
		return err
	}
	batchSize := vectorBatchSize(dimension)
	for start := 0; start < len(points); start += batchSize {
		batch := points[start:min(start+batchSize, len(points))]
		ids, vectors, metadata := make([]string, 0, len(batch)), make([][]float32, 0, len(batch)), make([]map[string]string, 0, len(batch))
		for _, point := range batch {
			ids = append(ids, vectorUUID(tenant, base, document, attempt, point.ID))
			vectors = append(vectors, point.Vector)
			row := vectorScope(tenant, base, document, attempt)
			row["chunk_id"] = point.ID
			metadata = append(metadata, row)
		}
		if _, err := v.request(ctx, store, http.MethodPost, "/collections/"+url.PathEscape(collection.ID)+"/upsert", map[string]any{"ids": ids, "embeddings": vectors, "metadatas": metadata}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (v *chromaDriver) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	return v.delete(ctx, store, tenant, base, document, "", false)
}

func (v *chromaDriver) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	return v.delete(ctx, store, tenant, base, document, attempt, true)
}

func (v *chromaDriver) delete(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, exact bool) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, exact); err != nil {
		return err
	}
	c, err := decodeChroma(store)
	if err != nil {
		return err
	}
	collections, err := v.collections(ctx, store)
	if err != nil {
		return err
	}
	for _, collection := range collections {
		dimension, err := strconv.Atoi(strings.TrimPrefix(collection.Name, c.collection+"_"))
		if err != nil || dimension < 1 || dimension > 65536 || collection.Name != c.collection+"_"+strconv.Itoa(dimension) {
			continue
		}
		if collection.ID == "" {
			return errors.New("Chroma 返回了缺少标识的集合")
		}
		status, err := v.request(ctx, store, http.MethodPost, "/collections/"+url.PathEscape(collection.ID)+"/delete", map[string]any{"where": chromaWhere(tenant, base, document, attempt)}, nil)
		if err != nil && status != http.StatusNotFound {
			return err
		}
	}
	return nil
}

func (v *chromaDriver) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(tenant, base, query, limit); err != nil {
		return nil, err
	}
	collection, exists, err := v.collection(ctx, store, len(query), false)
	if err != nil || !exists {
		return []port.VectorHit{}, err
	}
	var result struct {
		IDs       [][]string            `json:"ids"`
		Distances [][]float64           `json:"distances"`
		Metadata  [][]map[string]string `json:"metadatas"`
	}
	_, err = v.request(ctx, store, http.MethodPost, "/collections/"+url.PathEscape(collection.ID)+"/query", map[string]any{"query_embeddings": [][]float32{query}, "n_results": limit, "where": chromaWhere(tenant, base, "", ""), "include": []string{"metadatas", "distances"}}, &result)
	if err != nil {
		return nil, err
	}
	if len(result.IDs) != 1 || len(result.Distances) != 1 || len(result.Metadata) != 1 || len(result.IDs[0]) != len(result.Distances[0]) || len(result.IDs[0]) != len(result.Metadata[0]) || len(result.IDs[0]) > limit {
		return nil, errors.New("Chroma 返回的检索结果形状无效")
	}
	hits := make([]port.VectorHit, 0, len(result.IDs[0]))
	for i, metadata := range result.Metadata[0] {
		hit, err := vectorScopedHit(metadata, tenant, base, metadata["chunk_id"], 1-result.Distances[0][i])
		if err != nil {
			return nil, err
		}
		if result.IDs[0][i] != vectorUUID(tenant, base, metadata["document_id"], metadata["attempt"], hit.ID) {
			return nil, fmt.Errorf("Chroma 返回的记录标识与归属不匹配")
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
