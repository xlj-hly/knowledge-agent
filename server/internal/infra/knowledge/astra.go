package knowledge

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/httpclient"
)

//go:embed assets/astra.png
var astraLogo []byte

// AstraEntry 将 Astra DB Serverless Data API 连接与实际 collection 驱动关联。
func AstraEntry() Entry {
	return Entry{Definition: dto.VectorProvider{ID: "astra", Name: "Astra DB", Description: "Astra DB Serverless Data API · 外部 Embedding · cosine 集合", Logo: "data:image/png;base64," + base64.StdEncoding.EncodeToString(astraLogo), Protocol: "astra", Fields: []dto.VectorField{
		{Key: "endpoint", Label: "API Endpoint", Type: "text", Required: true, Immutable: true, Placeholder: "https://database-id-region.apps.astra.datastax.com"},
		{Key: "namespace", Label: "Keyspace", Type: "text", Required: true, Immutable: true, Default: "default_keyspace"},
		{Key: "collection", Label: "集合前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge"},
		{Key: "api_key", Label: "Application Token", Type: "password", Required: true, Secret: true},
	}}, Driver: &astraDriver{client: httpclient.New()}, Validate: validateAstraStore, Normalize: normalizeVectorEndpoint}
}

type astraConfig struct{ namespace, collection, key string }
type astraDriver struct{ client *http.Client }

func decodeAstra(store domain.VectorStore) (astraConfig, error) {
	c := astraConfig{namespace: store.Config["namespace"], collection: store.Config["collection"], key: store.Config["api_key"]}
	if err := domain.ValidateEndpoint(store.Config["endpoint"], false); err != nil {
		return c, err
	}
	if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,40}$`).MatchString(c.collection) || !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,47}$`).MatchString(c.namespace) || c.key == "" {
		return c, saas.Invalid("Astra DB 的 Keyspace、集合前缀或 Application Token 无效")
	}
	return c, nil
}

func validateAstraStore(store domain.VectorStore) error { _, err := decodeAstra(store); return err }

func (v *astraDriver) request(ctx context.Context, store domain.VectorStore, collection string, input, output any) error {
	c, err := decodeAstra(store)
	if err != nil {
		return err
	}
	path := "/api/json/v1/" + url.PathEscape(c.namespace)
	if collection != "" {
		path += "/" + url.PathEscape(collection)
	}
	var raw json.RawMessage
	if _, err := vectorJSON(ctx, v.client, store, http.MethodPost, path, map[string]string{"Token": c.key}, input, &raw); err != nil {
		return err
	}
	var envelope struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return errors.New("Astra DB 返回无效响应")
	}
	if len(envelope.Errors) > 0 {
		// Data API 会在 HTTP 200 中携带失败，不能把任意错误详情（可能回显 Token）传播出去。
		return errors.New("Astra DB Data API 操作失败")
	}
	if output != nil {
		if err := json.Unmarshal(raw, output); err != nil {
			return errors.New("Astra DB 返回无效响应结构")
		}
	}
	return nil
}

type astraCollection struct {
	Name    string `json:"name"`
	Options struct {
		Vector struct {
			Dimension int    `json:"dimension"`
			Metric    string `json:"metric"`
		} `json:"vector"`
	} `json:"options"`
}

func (v *astraDriver) collections(ctx context.Context, store domain.VectorStore) ([]astraCollection, error) {
	var result struct {
		Status struct {
			Collections *[]astraCollection `json:"collections"`
		} `json:"status"`
	}
	if err := v.request(ctx, store, "", map[string]any{"findCollections": map[string]any{"options": map[string]bool{"explain": true}}}, &result); err != nil {
		return nil, err
	}
	if result.Status.Collections == nil {
		return nil, errors.New("Astra DB 返回的集合列表无效")
	}
	return *result.Status.Collections, nil
}

func (v *astraDriver) Test(ctx context.Context, store domain.VectorStore) error {
	_, err := v.collections(ctx, store)
	return err
}

func (v *astraDriver) collection(ctx context.Context, store domain.VectorStore, dimension int, create bool) (string, bool, error) {
	c, err := decodeAstra(store)
	if err != nil {
		return "", false, err
	}
	name := c.collection + "_" + strconv.Itoa(dimension)
	for pass := 0; pass < 2; pass++ {
		collections, err := v.collections(ctx, store)
		if err != nil {
			return "", false, err
		}
		for _, collection := range collections {
			if collection.Name == name {
				if collection.Options.Vector.Dimension != dimension || collection.Options.Vector.Metric != "cosine" {
					return "", false, errors.New("Astra DB 集合维度或距离度量不匹配")
				}
				return name, true, nil
			}
		}
		if !create || pass > 0 {
			return name, false, nil
		}
		var result struct {
			Status struct {
				OK int `json:"ok"`
			} `json:"status"`
		}
		if err := v.request(ctx, store, "", map[string]any{"createCollection": map[string]any{"name": name, "options": map[string]any{"vector": map[string]any{"dimension": dimension, "metric": "cosine"}}}}, &result); err != nil {
			return "", false, err
		}
		if result.Status.OK != 1 {
			return "", false, errors.New("Astra DB 集合创建未确认成功")
		}
	}
	return "", false, errors.New("Astra DB 集合创建后不可见")
}

func (v *astraDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
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
	collection, exists, err := v.collection(ctx, store, dimension, true)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("Astra DB 集合创建后不可见")
	}
	if err := v.DeleteAttempt(ctx, store, tenant, base, document, attempt); err != nil {
		return err
	}
	for start := 0; start < len(points); start += 20 {
		batch := points[start:min(start+20, len(points))]
		rows := make([]map[string]any, 0, len(batch))
		for _, point := range batch {
			row := map[string]any{"_id": vectorUUID(tenant, base, document, attempt, point.ID), "$vector": point.Vector, "chunk_id": point.ID}
			for key, value := range vectorScope(tenant, base, document, attempt) {
				row[key] = value
			}
			rows = append(rows, row)
		}
		var result struct {
			Status struct {
				IDs []json.RawMessage `json:"insertedIds"`
			} `json:"status"`
		}
		if err := v.request(ctx, store, collection, map[string]any{"insertMany": map[string]any{"documents": rows, "options": map[string]bool{"ordered": true}}}, &result); err != nil {
			return err
		}
		if len(result.Status.IDs) != len(rows) {
			return errors.New("Astra DB 批量写入未全部完成")
		}
	}
	return nil
}

func (v *astraDriver) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	return v.delete(ctx, store, tenant, base, document, "", false)
}

func (v *astraDriver) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	return v.delete(ctx, store, tenant, base, document, attempt, true)
}

func (v *astraDriver) delete(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, exact bool) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, exact); err != nil {
		return err
	}
	c, err := decodeAstra(store)
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
		for page := 0; ; page++ {
			var result struct {
				Status struct {
					Deleted *int `json:"deletedCount"`
					More    bool `json:"moreData"`
				} `json:"status"`
			}
			if err := v.request(ctx, store, collection.Name, map[string]any{"deleteMany": map[string]any{"filter": vectorScope(tenant, base, document, attempt)}}, &result); err != nil {
				return err
			}
			if result.Status.Deleted == nil || *result.Status.Deleted < 0 {
				return errors.New("Astra DB 删除响应无效")
			}
			if !result.Status.More {
				break
			}
			if *result.Status.Deleted == 0 || page >= 1000 {
				return errors.New("Astra DB 批量删除未完成")
			}
		}
	}
	return nil
}

func (v *astraDriver) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(tenant, base, query, limit); err != nil {
		return nil, err
	}
	collection, exists, err := v.collection(ctx, store, len(query), false)
	if err != nil || !exists {
		return []port.VectorHit{}, err
	}
	var result struct {
		Data struct {
			Documents *[]struct {
				ID         string   `json:"_id"`
				Tenant     string   `json:"tenant_id"`
				Base       string   `json:"knowledge_base_id"`
				Document   string   `json:"document_id"`
				Attempt    string   `json:"attempt"`
				Chunk      string   `json:"chunk_id"`
				Similarity *float64 `json:"$similarity"`
			} `json:"documents"`
		} `json:"data"`
	}
	if err := v.request(ctx, store, collection, map[string]any{"find": map[string]any{"filter": vectorScope(tenant, base, "", ""), "sort": map[string]any{"$vector": query}, "options": map[string]any{"limit": limit, "includeSimilarity": true}, "projection": map[string]int{"$vector": 0}}}, &result); err != nil {
		return nil, err
	}
	if result.Data.Documents == nil || len(*result.Data.Documents) > limit {
		return nil, errors.New("Astra DB 检索响应无效")
	}
	hits := make([]port.VectorHit, 0, len(*result.Data.Documents))
	for _, row := range *result.Data.Documents {
		if row.Similarity == nil || *row.Similarity < 0 || *row.Similarity > 1 || row.ID != vectorUUID(tenant, base, row.Document, row.Attempt, row.Chunk) {
			return nil, errors.New("Astra DB 返回无效的记录标识或相似度")
		}
		metadata := vectorScope(row.Tenant, row.Base, row.Document, row.Attempt)
		metadata["chunk_id"] = row.Chunk
		// Data API 的 cosine similarity 为 (1 + cos) / 2，转回统一的原始 cosine 分数。
		hit, err := vectorScopedHit(metadata, tenant, base, row.Chunk, 2**row.Similarity-1)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
