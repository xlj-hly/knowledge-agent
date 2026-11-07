package repository

import (
	"context"
	"encoding/json"
	"unicode"

	"go-server/internal/domain/knowledge"
	"go-server/internal/infra/database"
)

// KeywordSearch 在企业、库和当前完成代内执行正文词法检索；不把标题或元数据当正文。
// PostgreSQL simple 不拆中文连续句，因此以重叠双字 gram 补充召回，按去重后的覆盖率排序。
// 至少匹配两个不同 gram，防止仅共有“是谁”等一个词片段就召回；双字短查询仍可匹配一个。
// 单字及标点保持字面查询，英文保留 PostgreSQL 全文匹配；所有输入均参数绑定。
func (s *Store) KeywordSearch(ctx context.Context, t, b, query string, limit int) ([]knowledge.Hit, error) {
	grams := hanBigrams(query)
	encoded, err := json.Marshal(grams)
	if err != nil {
		return nil, checked("encode keyword terms", err)
	}
	minimum := min(2, len(grams))
	if minimum == 0 {
		minimum = 1
	}
	// 这里只扫描符合知识库归属和当前代约束的候选。query 已由用例限制为 2000 字，
	// gram 数量随输入线性有界；当前未持久化中文倒排索引，不能据此宣称大规模索引性能。
	q := hitQuery(database.Connection(ctx, s.db).WithContext(ctx)).Where("c.tenant_id = ? AND c.base_id = ?", t, b).
		Joins(`JOIN LATERAL (
			SELECT count(*) AS matches
			FROM jsonb_array_elements_text(?::jsonb) AS gram(value)
			WHERE strpos(c.content, gram.value) > 0
		) lexical ON true`, string(encoded)).
		Select(`c.*,d.title AS document_title,d.file_name,
			GREATEST(
				CASE WHEN strpos(lower(c.content),lower(?)) > 0 THEN 1.0 ELSE 0 END,
				ts_rank_cd(to_tsvector('simple',c.content),plainto_tsquery('simple',?)),
				CASE WHEN lexical.matches >= ? THEN 0.9*lexical.matches/? ELSE 0 END
			) AS score`, query, query, minimum, max(1, len(grams))).
		Where(`to_tsvector('simple',c.content) @@ plainto_tsquery('simple',?)
			OR strpos(lower(c.content),lower(?)) > 0 OR lexical.matches >= ?`, query, query, minimum).
		Order("score DESC,c.id ASC").Limit(limit)
	rows := []hitRow{}
	err = q.Scan(&rows).Error
	return hits(rows), checked("keyword search", err)
}

// hanBigrams 只组合相邻 Han 字符，不跨标点、空白或其他文字；重复 gram 不重复计分。
// 采用与 WeKnora SQLite 检索一致的双字词法思路，不依赖运行时词典或数据库扩展。
func hanBigrams(text string) []string {
	grams := []string{}
	seen := map[string]bool{}
	var previous rune
	for _, current := range text {
		if !unicode.Is(unicode.Han, current) {
			previous = 0
			continue
		}
		if previous != 0 {
			gram := string([]rune{previous, current})
			if !seen[gram] {
				seen[gram] = true
				grams = append(grams, gram)
			}
		}
		previous = current
	}
	return grams
}
