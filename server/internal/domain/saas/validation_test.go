package domain

import "testing"

// A lax host parser could turn a malformed enterprise address into another tenant.
func TestNormalizeHostname(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"ACME.Example.com:5173", "acme.example.com"},
		{"acme.localhost", "acme.localhost"},
		{"https://acme.test/path", ""},
		{"bad host", ""}, {"a..test", ""}, {"-bad.test", ""},
		{"evil.test/path", ""}, {"", ""}, {"a.test:abc", ""},
	} {
		got, err := NormalizeHostname(tt.input)
		if tt.want == "" && err == nil {
			t.Errorf("%q should be rejected", tt.input)
		}
		if tt.want != "" && (err != nil || got != tt.want) {
			t.Errorf("%q got %q, %v; want %q", tt.input, got, err, tt.want)
		}
	}
}

func TestAccountValidation(t *testing.T) {
	if got, err := NormalizeUsername("Alice_01"); err != nil || got != "alice_01" {
		t.Fatalf("username: %q %v", got, err)
	}
	for _, input := range []string{"ab", "admin space", "../../admin", "管理员"} {
		if _, err := NormalizeUsername(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	if err := ValidatePassword("0123456789"); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("accepted short password")
	}
	if err := ValidateAvatar("javascript:alert(1)"); err == nil {
		t.Fatal("accepted script avatar")
	}
}
