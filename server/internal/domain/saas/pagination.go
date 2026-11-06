package domain

// Pagination 表示已解析的分页与搜索条件，使用前仍需调用 Validate。
type Pagination struct {
	Page  int
	Size  int
	Query string
}

// Page 返回非 nil 的 Items 和匹配总数，Page、Size 反映本次分页参数。
type Page[T any] struct {
	Items []T
	Total int64
	Page  int
	Size  int
}

// Validate 检查分页与搜索范围，拒绝无效值而不静默截断或调整。
func (p Pagination) Validate() error {
	if p.Page < 1 || p.Page > 1000000 || p.Size < 1 || p.Size > 100 || len(p.Query) > 128 {
		return Invalid("分页参数无效")
	}
	return nil
}
