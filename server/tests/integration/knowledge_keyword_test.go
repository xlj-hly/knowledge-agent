package saas_test

import (
	"context"
	"strings"
	"testing"

	kd "go-server/internal/app/knowledge/dto"
	kr "go-server/internal/infra/persistence/knowledge"
)

func TestKnowledgeKeywordNaturalChineseQueries(t *testing.T) {
	s := newIntegrationSuite(t)
	alpha := s.createTenant(t, "keyword.alpha.test")
	s.createTenant(t, "keyword.beta.test")
	a := s.login(t, "keyword.alpha.test", "tenant.admin", adminPassword)
	b := s.login(t, "keyword.beta.test", "tenant.admin", adminPassword)
	createBase := func(host, token string) kd.Base {
		return decodeSuccess[kd.Base](t, s.request("POST", host, "/api/knowledge/bases", token, map[string]any{"name": "检索资料", "indexing_mode": "keyword", "retrieval_mode": "keyword", "chunk_size": 512, "chunk_overlap": 20}))
	}
	base := createBase("keyword.alpha.test", a.Token)
	other := createBase("keyword.beta.test", b.Token)
	add := func(host, token, baseID, title, content string) kd.Document {
		doc := decodeSuccess[kd.Document](t, s.request("POST", host, "/api/knowledge/bases/"+baseID+"/documents", token, map[string]any{"title": title, "content": content}))
		knowledgeProcess(t, s)
		return doc
	}
	target := add("keyword.alpha.test", a.Token, base.ID, "项目规则", "航星计划的审批人是林沐。财务核对人为许澄。")
	add("keyword.alpha.test", a.Token, base.ID, "生态资料", "海洋生态监测关注珊瑚生长与清洁水质。")
	add("keyword.alpha.test", a.Token, base.ID, "日常用语", "请说明你是谁，然后阅读日常用语练习。")
	english := add("keyword.alpha.test", a.Token, base.ID, "Travel policy", "Travel reimbursement requires an itemized receipt.")
	add("keyword.beta.test", b.Token, other.ID, "其他企业项目", "航星计划的审批人是赵青。")

	search := func(query string) []kd.Hit {
		return decodeSuccess[[]kd.Hit](t, s.request("POST", "keyword.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": query, "mode": "keyword"}))
	}
	for _, query := range []string{"航星计划的审批人是谁？", "林沐", "沐"} {
		hits := search(query)
		if len(hits) != 1 || hits[0].DocumentID != target.ID || !strings.Contains(hits[0].Content, "林沐") {
			t.Fatalf("query %q did not retrieve only the current company fact: %+v", query, hits)
		}
	}
	if hits := search("火山喷发监测方法"); len(hits) != 0 {
		t.Fatalf("unrelated Chinese question matched: %+v", hits)
	}
	if hits := search("REIMBURSEMENT receipt"); len(hits) != 1 || hits[0].DocumentID != english.ID {
		t.Fatalf("English multiword full-text search regressed: %+v", hits)
	}
	if hits := search("%_ | ' !"); len(hits) != 0 {
		t.Fatalf("search punctuation expanded into arbitrary matches: %+v", hits)
	}
	assertFailure(t, s.request("POST", "keyword.beta.test", "/api/knowledge/bases/"+base.ID+"/search", b.Token, map[string]any{"query": "航星计划的审批人是谁？"}), 404)

	// 故意保留一个旧处理代候选，验证新召回分支也受同一当前代 JOIN 约束。
	repo := kr.New(s.db)
	chunks, err := repo.Chunks(context.Background(), alpha.Tenant.ID, target.ID)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("read fixture chunks: count=%d err=%v", len(chunks), err)
	}
	stale := chunks[0]
	stale.ID, stale.Attempt = "9223372036854775000", "obsolete-keyword-generation"
	stale.Content = "航星计划的审批人是谁？错误旧代不应召回。"
	if err := repo.SaveChunk(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	if hits := search("航星计划的审批人是谁？"); len(hits) != 1 || hits[0].DocumentID != target.ID || hits[0].ID == stale.ID {
		t.Fatalf("obsolete generation entered keyword search: %+v", hits)
	}
	decodeSuccess[kd.Document](t, s.request("POST", "keyword.alpha.test", "/api/knowledge/documents/"+target.ID+"/reprocess", a.Token, nil))
	if hits := search("航星计划的审批人是谁？"); len(hits) != 0 {
		t.Fatalf("queued document remained visible in keyword search: %+v", hits)
	}
}
