package ai

import "testing"

func TestModelCapabilityValidation(t *testing.T) {
	m := Model{Name: "企业模型", Kind: "chat", Provider: "openai-compatible", Endpoint: "http://localhost:9000/v1", ModelName: "example"}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	// 新增服务商不要求领域层维护品牌名单，注册与用途由能力目录检查。
	m.Provider = "new-company-gateway"
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	m.Kind = "embedding"
	if err := m.Validate(); err == nil {
		t.Fatal("embedding accepted without dimension")
	}
	m.Dimension = 1024
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	m.Endpoint = "https://secret@example.com/v1"
	if err := m.Validate(); err == nil {
		t.Fatal("credentials embedded in endpoint")
	}
}

func TestSettingsRejectInvalidGenerationAndIndexParameters(t *testing.T) {
	s := DefaultSettings()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.ChunkOverlap = s.ChunkSize
	if err := s.Validate(); err == nil {
		t.Fatal("invalid overlap accepted")
	}
	s = DefaultSettings()
	s.Temperature = 3
	if err := s.Validate(); err == nil {
		t.Fatal("invalid temperature accepted")
	}
	s = DefaultSettings()
	s.RetrievalMode = "automatic-fallback"
	if err := s.Validate(); err == nil {
		t.Fatal("unknown retrieval mode accepted")
	}
}
