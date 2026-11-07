package chat

import "testing"

func TestSessionRejectsInvalidConfiguration(t *testing.T) {
	base := Session{Title: "新对话", ModelID: "1", KnowledgeBaseIDs: []string{}, SystemPrompt: "请根据资料回答", Temperature: .7, MaxTokens: 2048, HistoryTurns: 10, RetrievalMode: "hybrid", RetrievalLimit: 6}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Session){
		func(s *Session) { s.KnowledgeBaseIDs = []string{"1", "1"} },
		func(s *Session) { s.Temperature = -1 },
		func(s *Session) { s.RetrievalMode = "unknown" },
		func(s *Session) { s.MaxTokens = 0 },
		func(s *Session) { s.SystemPrompt = "\x00" },
	} {
		candidate := base
		change(&candidate)
		if candidate.Validate() == nil {
			t.Fatal("invalid session was accepted")
		}
	}
}

func TestValidateContentBounds(t *testing.T) {
	for _, input := range []string{"", "   ", "hello\x00world"} {
		if ValidateContent(input) == nil {
			t.Fatalf("invalid content accepted: %q", input)
		}
	}
	if err := ValidateContent("这个项目的负责人是谁？"); err != nil {
		t.Fatal(err)
	}
}
