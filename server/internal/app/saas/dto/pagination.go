package dto

import "go-server/internal/domain/saas"

// Pagination 表示已解析的分页与搜索条件，使用前仍需调用 Validate。
type Pagination struct {
	Page  int    `json:"page"`
	Size  int    `json:"page_size"`
	Query string `json:"-"`
}

// Page 返回非 nil 的 Items 和匹配总数，Page、Size 反映本次分页参数。
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"page_size"`
}

// Validate 检查分页范围，领域仓储使用相同的受控范围。
func (p Pagination) Validate() error { return p.Domain().Validate() }

// Domain 将用例查询转换为领域查询，不传递输出契约。
func (p Pagination) Domain() domain.Pagination {
	return domain.Pagination{Page: p.Page, Size: p.Size, Query: p.Query}
}
