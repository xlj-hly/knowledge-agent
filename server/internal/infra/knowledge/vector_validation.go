package knowledge

import (
	"errors"
	"math"
)

// validateVectors 在向量存储适配边界拒绝无效维度、非有限值及零向量。
func validateVectors(vectors [][]float32, count, dimension int) error {
	if len(vectors) != count || dimension < 1 {
		return errors.New("向量数量或维度无效")
	}
	for _, vector := range vectors {
		if len(vector) != dimension {
			return errors.New("向量维度不一致")
		}
		var magnitude float64
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return errors.New("向量包含非有限值")
			}
			magnitude += float64(value) * float64(value)
		}
		if magnitude == 0 {
			return errors.New("不允许写入零向量")
		}
	}
	return nil
}
