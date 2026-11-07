package presets

import (
	"strings"
	"sync"
	"testing"

	"go-server/internal/app/ai/dto"
	domain "go-server/internal/domain/ai"
)

func testDefinitions() dto.Presets {
	return dto.Presets{
		Prompts:         []dto.PromptPreset{{ID: "custom", Name: "自定义助手", Content: "根据资料回答。"}},
		Chunking:        []dto.ChunkPreset{{ID: "custom", Name: "自定义文档", Strategy: "auto", Size: 256, Overlap: 32}},
		ChunkStrategies: []dto.ChunkStrategy{{ID: "auto", Name: "自动识别", Description: "根据文档结构切分。"}},
	}
}

func TestCatalogRejectsInvalidDefinitions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*dto.Presets)
	}{
		{"empty prompt ID", func(d *dto.Presets) { d.Prompts[0].ID = "" }},
		{"invalid ID", func(d *dto.Presets) { d.Prompts[0].ID = "custom\x00" }},
		{"duplicate prompt ID", func(d *dto.Presets) { d.Prompts = append(d.Prompts, d.Prompts[0]) }},
		{"empty prompt name", func(d *dto.Presets) { d.Prompts[0].Name = " " }},
		{"NUL prompt name", func(d *dto.Presets) { d.Prompts[0].Name = "hello\x00" }},
		{"empty prompt content", func(d *dto.Presets) { d.Prompts[0].Content = " " }},
		{"oversized prompt content", func(d *dto.Presets) { d.Prompts[0].Content = strings.Repeat("x", 16001) }},
		{"NUL prompt content", func(d *dto.Presets) { d.Prompts[0].Content = "hello\x00" }},
		{"empty chunk ID", func(d *dto.Presets) { d.Chunking[0].ID = "" }},
		{"duplicate chunk ID", func(d *dto.Presets) { d.Chunking = append(d.Chunking, d.Chunking[0]) }},
		{"empty chunk name", func(d *dto.Presets) { d.Chunking[0].Name = "" }},
		{"unregistered strategy", func(d *dto.Presets) { d.Chunking[0].Strategy = "recursive" }},
		{"invalid chunk size", func(d *dto.Presets) { d.Chunking[0].Size = 1 }},
		{"invalid overlap", func(d *dto.Presets) { d.Chunking[0].Overlap = 256 }},
		{"empty strategy ID", func(d *dto.Presets) { d.ChunkStrategies[0].ID = "" }},
		{"duplicate strategy ID", func(d *dto.Presets) { d.ChunkStrategies = append(d.ChunkStrategies, d.ChunkStrategies[0]) }},
		{"unsupported algorithm", func(d *dto.Presets) { d.ChunkStrategies[0].ID = "new-algorithm" }},
		{"empty strategy name", func(d *dto.Presets) { d.ChunkStrategies[0].Name = "" }},
		{"empty strategy description", func(d *dto.Presets) { d.ChunkStrategies[0].Description = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definitions := testDefinitions()
			tc.change(&definitions)
			if _, err := New(definitions); err == nil {
				t.Fatal("invalid product preset accepted")
			}
		})
	}
}

func TestBuiltinCatalogPreservesProductDefaultsAndStrategyOrder(t *testing.T) {
	catalog, err := NewBuiltin()
	if err != nil {
		t.Fatal(err)
	}
	presets := catalog.Presets()
	if len(presets.Prompts) != 2 || presets.Prompts[0].ID != "assistant" || presets.Prompts[0].Content != domain.DefaultPrompt {
		t.Fatalf("unexpected prompt presets: %v", presets.Prompts)
	}
	if len(presets.Chunking) != 3 || presets.Chunking[0].Strategy != "auto" || presets.Chunking[0].Size != 512 || presets.Chunking[0].Overlap != 80 {
		t.Fatalf("unexpected chunk presets: %v", presets.Chunking)
	}
	for i, want := range []struct{ id, name string }{{"auto", "自动识别"}, {"recursive", "递归切分"}, {"heading", "按标题结构"}} {
		if len(presets.ChunkStrategies) != 3 || presets.ChunkStrategies[i].ID != want.id || presets.ChunkStrategies[i].Name != want.name {
			t.Fatalf("unexpected strategy order: %v", presets.ChunkStrategies)
		}
	}
}

func TestCatalogOwnsDefinitionsAndDoesNotShareState(t *testing.T) {
	definitions := testDefinitions()
	catalog, err := New(definitions)
	if err != nil {
		t.Fatal(err)
	}
	other, err := New(definitions)
	if err != nil {
		t.Fatal(err)
	}
	definitions.Prompts[0].Content = "changed input"
	definitions.Chunking[0].Size = 128
	definitions.ChunkStrategies[0].Name = "changed input"
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				rows := catalog.Presets()
				if rows.Prompts[0].Content != "根据资料回答。" || rows.Chunking[0].Size != 256 || rows.ChunkStrategies[0].Name != "自动识别" {
					t.Error("catalog changed through external definitions")
				}
				rows.Prompts[0].Content = "changed output"
				rows.Chunking[0].Size = 128
				rows.ChunkStrategies[0].Name = "changed output"
			}
		})
	}
	wg.Wait()
	if other.Presets().Prompts[0].Content != "根据资料回答。" || domain.DefaultSettings().ChunkSize != 512 {
		t.Fatal("catalog altered another instance or product defaults")
	}
}
