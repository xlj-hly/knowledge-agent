package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go-server/internal/infra/httpclient"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"go-server/internal/app/knowledge/port"
	"go-server/internal/domain/knowledge"
)

var collectionPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

// qdrantDriver 实现 Qdrant REST 协议，不承担引擎注册或企业连接管理。
type qdrantDriver struct{ client *http.Client }

// newQdrantDriver 创建有界 HTTP 客户端，连接信息来自每次已授权请求。
func newQdrantDriver() *qdrantDriver { return &qdrantDriver{client: httpclient.New()} }

func (v *qdrantDriver) request(ctx context.Context, store knowledge.VectorStore, method, path string, input, output any) (int, error) {
	if store.Provider != "qdrant" || !collectionPattern.MatchString(store.Config["collection"]) {
		return 0, errors.New("向量存储类型或集合前缀无效")
	}
	address, err := httpclient.Endpoint(store.Config["endpoint"], "")
	if err != nil {
		return 0, err
	}
	headers := map[string]string{}
	if store.Config["api_key"] != "" {
		headers["api-key"] = store.Config["api_key"]
	}
	status, err := httpclient.JSON(ctx, v.client, method, address+path, headers, input, output, 8<<20)
	if err != nil {
		return status, fmt.Errorf("Qdrant 请求失败: %w", err)
	}
	return status, nil
}

func (v *qdrantDriver) collections(ctx context.Context, store knowledge.VectorStore) ([]string, error) {
	var output struct {
		Result *struct {
			Collections []struct {
				Name string `json:"name"`
			} `json:"collections"`
		} `json:"result"`
	}
	if _, err := v.request(ctx, store, http.MethodGet, "/collections", nil, &output); err != nil {
		return nil, err
	}
	if output.Result == nil {
		return nil, errors.New("Qdrant 返回的集合列表无效")
	}
	names := make([]string, 0, len(output.Result.Collections))
	for _, collection := range output.Result.Collections {
		names = append(names, collection.Name)
	}
	return names, nil
}

// Test 检查页面配置是否可连接并使用正确的 Qdrant 协议。
func (v *qdrantDriver) Test(ctx context.Context, store knowledge.VectorStore) error {
	_, err := v.collections(ctx, store)
	return err
}

func collectionName(store knowledge.VectorStore, dimension int) string {
	return store.Config["collection"] + "_" + strconv.Itoa(dimension)
}

func (v *qdrantDriver) ensureCollection(ctx context.Context, store knowledge.VectorStore, dimension int) error {
	return v.ensureCollectionOnce(ctx, store, dimension, true)
}

func (v *qdrantDriver) ensureCollectionOnce(ctx context.Context, store knowledge.VectorStore, dimension int, allowCreate bool) error {
	name := collectionName(store, dimension)
	var output struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size     int    `json:"size"`
						Distance string `json:"distance"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	status, err := v.request(ctx, store, http.MethodGet, "/collections/"+name, nil, &output)
	if err == nil {
		if output.Result.Config.Params.Vectors.Size != dimension || !strings.EqualFold(output.Result.Config.Params.Vectors.Distance, "Cosine") {
			return errors.New("Qdrant 现有集合的向量维度或距离度量与配置不一致")
		}
		return nil
	}
	if status != http.StatusNotFound || !allowCreate {
		return err
	}
	input := map[string]any{"vectors": map[string]any{"size": dimension, "distance": "Cosine"}}
	status, err = v.request(ctx, store, http.MethodPut, "/collections/"+name, input, nil)
	if err != nil && status == http.StatusConflict {
		// 两个 worker 可能同时创建同维度集合；确认胜出者的配置，而非吞掉创建错误。
		return v.ensureCollectionOnce(ctx, store, dimension, false)
	}
	return err
}

type qdrantPoint struct {
	ID      uint64            `json:"id"`
	Vector  []float32         `json:"vector"`
	Payload map[string]string `json:"payload"`
}

// ReplaceDocument 仅写入该处理代，旧代清理由应用层完成状态确认后显式发起。
func (v *qdrantDriver) ReplaceDocument(ctx context.Context, store knowledge.VectorStore, tenantID, baseID, documentID, attempt string, points []port.VectorPoint) error {
	if len(points) == 0 {
		return nil
	}
	if tenantID == "" || baseID == "" || documentID == "" || attempt == "" {
		return errors.New("向量索引缺少所属范围")
	}
	dimension := len(points[0].Vector)
	if dimension < 1 || dimension > 65536 {
		return errors.New("索引向量维度无效")
	}
	items := make([]qdrantPoint, len(points))
	for i, point := range points {
		id, err := strconv.ParseUint(point.ID, 10, 64)
		if err != nil || id == 0 {
			return errors.New("分块 ID 不能用作向量索引 ID")
		}
		if err := validateVectors([][]float32{point.Vector}, 1, dimension); err != nil {
			return err
		}
		items[i] = qdrantPoint{ID: id, Vector: point.Vector, Payload: map[string]string{
			"tenant_id": tenantID, "knowledge_base_id": baseID,
			"document_id": documentID, "attempt": attempt, "chunk_id": point.ID,
		}}
	}
	if err := v.ensureCollection(ctx, store, dimension); err != nil {
		return err
	}
	for start := 0; start < len(items); start += 64 {
		input := map[string]any{"points": items[start:min(start+64, len(items))]}
		if _, err := v.request(ctx, store, http.MethodPut, "/collections/"+collectionName(store, dimension)+"/points?wait=true", input, nil); err != nil {
			return err
		}
	}
	return nil
}

func scopeFilter(tenantID, baseID, documentID, attempt string) map[string]any {
	conditions := []map[string]any{
		{"key": "tenant_id", "match": map[string]string{"value": tenantID}},
		{"key": "knowledge_base_id", "match": map[string]string{"value": baseID}},
	}
	if documentID != "" {
		conditions = append(conditions, map[string]any{"key": "document_id", "match": map[string]string{"value": documentID}})
	}
	if attempt != "" {
		conditions = append(conditions, map[string]any{"key": "attempt", "match": map[string]string{"value": attempt}})
	}
	return map[string]any{"must": conditions}
}

// DeleteDocument 幂等删除一份资料的所有处理代，始终限定企业和知识库。
func (v *qdrantDriver) DeleteDocument(ctx context.Context, store knowledge.VectorStore, tenantID, baseID, documentID string) error {
	return v.delete(ctx, store, tenantID, baseID, documentID, "")
}

// DeleteAttempt 仅清理已失效的处理代，不能删除并发产生的新代。
func (v *qdrantDriver) DeleteAttempt(ctx context.Context, store knowledge.VectorStore, tenantID, baseID, documentID, attempt string) error {
	if attempt == "" {
		return errors.New("清理索引必须指定处理代")
	}
	return v.delete(ctx, store, tenantID, baseID, documentID, attempt)
}

func (v *qdrantDriver) delete(ctx context.Context, store knowledge.VectorStore, tenantID, baseID, documentID, attempt string) error {
	if tenantID == "" || baseID == "" || documentID == "" {
		return errors.New("清理索引缺少所属范围")
	}
	names, err := v.collections(ctx, store)
	if err != nil {
		return err
	}
	for _, name := range names {
		if !strings.HasPrefix(name, store.Config["collection"]+"_") {
			continue
		}
		dimension, err := strconv.Atoi(strings.TrimPrefix(name, store.Config["collection"]+"_"))
		if err != nil || dimension < 1 || dimension > 65536 || name != collectionName(store, dimension) {
			continue
		}
		input := map[string]any{"filter": scopeFilter(tenantID, baseID, documentID, attempt)}
		status, err := v.request(ctx, store, http.MethodPost, "/collections/"+name+"/points/delete?wait=true", input, nil)
		if err != nil && status != http.StatusNotFound {
			return err
		}
	}
	return nil
}

// Search 只返回当前企业与知识库内的索引命中；原文及当前代由业务仓储再次核实。
func (v *qdrantDriver) Search(ctx context.Context, store knowledge.VectorStore, tenantID, baseID string, query []float32, limit int) ([]port.VectorHit, error) {
	if tenantID == "" || baseID == "" || limit < 1 || limit > 200 || len(query) < 1 || len(query) > 65536 {
		return nil, errors.New("向量检索范围、维度或数量无效")
	}
	if err := validateVectors([][]float32{query}, 1, len(query)); err != nil {
		return nil, err
	}
	input := map[string]any{"vector": query, "limit": limit, "filter": scopeFilter(tenantID, baseID, "", ""), "with_payload": true}
	var output struct {
		Result []struct {
			ID      json.RawMessage   `json:"id"`
			Score   float64           `json:"score"`
			Payload map[string]string `json:"payload"`
		} `json:"result"`
	}
	status, err := v.request(ctx, store, http.MethodPost, "/collections/"+collectionName(store, len(query))+"/points/search", input, &output)
	if status == http.StatusNotFound {
		return []port.VectorHit{}, nil
	}
	if err != nil {
		return nil, err
	}
	hits := make([]port.VectorHit, 0, len(output.Result))
	for _, item := range output.Result {
		if item.Payload["tenant_id"] != tenantID || item.Payload["knowledge_base_id"] != baseID {
			return nil, errors.New("Qdrant 返回了不属于当前范围的索引结果")
		}
		id := item.Payload["chunk_id"]
		if _, err := strconv.ParseUint(id, 10, 64); err != nil || id == "0" || math.IsNaN(item.Score) || math.IsInf(item.Score, 0) {
			return nil, errors.New("Qdrant 返回了无效索引结果")
		}
		hits = append(hits, port.VectorHit{ID: id, Score: item.Score})
	}
	return hits, nil
}
