package knowledge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "go-server/internal/domain/knowledge"
)

func TestProcessorUsesDocReaderPDFProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read" {
			t.Errorf("wrong DocReader path: %s", r.URL.Path)
		}
		var req struct {
			Content string `json:"file_content"`
			Name    string `json:"file_name"`
			Type    string `json:"file_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		decoded, err := base64.StdEncoding.DecodeString(req.Content)
		if err != nil || string(decoded) != "%PDF-fixture" || req.Name != "guide.pdf" || req.Type != "pdf" {
			t.Errorf("incorrect PDF request: %+v", req)
		}
		_, _ = w.Write([]byte(`{"markdown_content":"# 退款政策\n\n七天内可以申请退款。"}`))
	}))
	defer server.Close()
	text, err := NewProcessor().Parse(context.Background(), "guide.pdf", []byte("%PDF-fixture"), server.URL)
	if err != nil || !strings.Contains(text, "七天") {
		t.Fatalf("DocReader result lost: %q %v", text, err)
	}
	if _, err := NewProcessor().Parse(context.Background(), "guide.pdf", []byte("%PDF-fixture"), ""); err == nil {
		t.Fatal("PDF without parser endpoint accepted")
	}
}

func TestProcessorPreservesUnicodeSourceOffsets(t *testing.T) {
	text := "# 售后说明\n\n" + strings.Repeat("购买后七日内可申请退款，收货后请保存完整包装。", 60) + "\n\n## 联系方式\n客服电话：4000001234。"
	chunks := NewProcessor().Split(text, domain.ChunkingConfig{Strategy: "auto", Size: 120, Overlap: 20})
	if len(chunks) < 2 {
		t.Fatalf("document was not split: %d", len(chunks))
	}
	runes := []rune(text)
	for i, chunk := range chunks {
		if chunk.Index != i || chunk.Start < 0 || chunk.End > len(runes) || chunk.End <= chunk.Start {
			t.Fatalf("invalid source range: %+v", chunk)
		}
		if got := string(runes[chunk.Start:chunk.End]); got != chunk.Content {
			t.Fatalf("source content differs at %d", i)
		}
	}
}

func TestProcessorCarriesHeadingContextWithoutChangingSource(t *testing.T) {
	text := "# 员工手册\n\n## 出差报销\n\n" + strings.Repeat("交通费用每日上限120元，报销须提交审批单。", 30)
	chunks := NewProcessor().Split(text, domain.ChunkingConfig{Strategy: "heading", Size: 160, Overlap: 20})
	found := false
	runes := []rune(text)
	for _, chunk := range chunks {
		if chunk.ContextHeader != "" {
			found = true
			if !strings.Contains(chunk.EmbeddingContent(), "出差报销") {
				t.Fatalf("标题上下文没有传给向量化正文: %q", chunk.EmbeddingContent())
			}
		}
		if string(runes[chunk.Start:chunk.End]) != chunk.Content {
			t.Fatal("标题上下文破坏了原文范围")
		}
	}
	if !found {
		t.Fatal("标题切分策略的上下文丢失")
	}
}
