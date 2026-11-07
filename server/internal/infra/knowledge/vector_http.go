package knowledge

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	"go-server/internal/infra/httpclient"
)

// vectorJSON 只使用当前连接的地址；所有驱动共用有界响应、取消及脱敏 HTTP 错误。
func vectorJSON(ctx context.Context, client *http.Client, store domain.VectorStore, method, path string, headers map[string]string, input, output any) (int, error) {
	address, err := httpclient.Endpoint(store.Config["endpoint"], "")
	if err != nil {
		return 0, err
	}
	status, err := httpclient.JSON(ctx, client, method, address+path, headers, input, output, 8<<20)
	if err != nil {
		return status, fmt.Errorf("向量服务请求失败: %w", err)
	}
	return status, nil
}

func normalizeVectorEndpoint(config map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(config))
	for key, value := range config {
		result[key] = value
	}
	if value, exists := result["endpoint"]; exists {
		result["endpoint"] = strings.TrimRight(strings.TrimSpace(value), "/")
	}
	return result, nil
}

func vectorScope(tenant, base, document, attempt string) map[string]string {
	result := map[string]string{"tenant_id": tenant, "knowledge_base_id": base}
	if document != "" {
		result["document_id"] = document
	}
	if attempt != "" {
		result["attempt"] = attempt
	}
	return result
}

func vectorRequiredScope(store domain.VectorStore, tenant, base, document, attempt string, requireAttempt bool) error {
	for _, value := range []string{tenant, base, document} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsRune(value, 0) {
			return errors.New("向量操作缺少有效的所属范围")
		}
	}
	if requireAttempt && (strings.TrimSpace(attempt) == "" || len(attempt) > 256 || strings.ContainsRune(attempt, 0)) {
		return errors.New("向量操作必须指定有效处理代")
	}
	if store.TenantID != "" && store.TenantID != tenant {
		return errors.New("向量连接不属于当前企业")
	}
	return nil
}

func vectorPoints(points []port.VectorPoint) (int, error) {
	if len(points) == 0 {
		return 0, nil
	}
	dimension := len(points[0].Vector)
	if dimension < 1 || dimension > 65536 {
		return 0, errors.New("索引向量维度无效")
	}
	seen := make(map[string]struct{}, len(points))
	for _, point := range points {
		if strings.TrimSpace(point.ID) == "" || len(point.ID) > 256 || strings.ContainsRune(point.ID, 0) {
			return 0, errors.New("分块 ID 无效")
		}
		if _, exists := seen[point.ID]; exists {
			return 0, errors.New("分块 ID 重复")
		}
		seen[point.ID] = struct{}{}
		if err := validateVectors([][]float32{point.Vector}, 1, dimension); err != nil {
			return 0, err
		}
	}
	return dimension, nil
}

func vectorSearchInput(tenant, base string, query []float32, limit int) error {
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(base) == "" || len(tenant) > 256 || len(base) > 256 || strings.ContainsRune(tenant+base, 0) || limit < 1 || limit > 200 || len(query) < 1 || len(query) > 65536 {
		return errors.New("向量检索范围、维度或数量无效")
	}
	return validateVectors([][]float32{query}, 1, len(query))
}

func vectorScopedHit(metadata map[string]string, tenant, base, id string, score float64) (port.VectorHit, error) {
	if metadata["tenant_id"] != tenant || metadata["knowledge_base_id"] != base || metadata["document_id"] == "" || metadata["attempt"] == "" {
		return port.VectorHit{}, errors.New("向量服务返回了不属于当前范围的索引结果")
	}
	if id == "" || metadata["chunk_id"] != id || len(id) > 256 || strings.ContainsRune(id, 0) || math.IsNaN(score) || math.IsInf(score, 0) {
		return port.VectorHit{}, errors.New("向量服务返回了无效索引结果")
	}
	// 驱动须先转换为原始 cosine；仅容忍浮点舍入误差，不能把协议转换错误截成合法分数。
	if score < -1.000001 || score > 1.000001 {
		return port.VectorHit{}, errors.New("向量服务返回的 cosine 分数超出有效范围")
	}
	return port.VectorHit{ID: id, Score: math.Max(-1, math.Min(1, score))}, nil
}

func vectorIdentifier(value string, max int) bool {
	if len(value) == 0 || len(value) > max || (value[0] < 'a' || value[0] > 'z') && (value[0] < 'A' || value[0] > 'Z') {
		return false
	}
	for _, c := range value {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

// vectorBatchSize 同时限制记录数与向量标量数，避免高维模型使固定条数批次膨胀成超大请求。
func vectorBatchSize(dimension int) int {
	if dimension < 1 {
		return 1
	}
	return max(1, min(64, (128<<10)/dimension))
}

// vectorUUID 用长度前缀编码避免范围拼接碰撞；不同处理代永远不覆盖相同业务分块的远端记录。
func vectorUUID(tenant, base, document, attempt, id string) string {
	hash := sha256.New()
	for _, value := range []string{tenant, base, document, attempt, id} {
		fmt.Fprintf(hash, "%d:%s", len(value), value)
	}
	value := hash.Sum(nil)[:16]
	value[6] = value[6]&0x0f | 0x50
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}
