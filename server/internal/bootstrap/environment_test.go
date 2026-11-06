package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvironmentFindsModuleConfigurationAndKeepsProcessOverrides(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(filepath.Join(root, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "server", ".env")
	if err := os.WriteFile(path, []byte("SAAS_ENV_FILE_VALUE=from-file\nSAAS_ENV_OVERRIDE=from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SAAS_ENV_FILE_VALUE", "")
	if err := os.Unsetenv("SAAS_ENV_FILE_VALUE"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SAAS_ENV_OVERRIDE", "from-process")
	if err := LoadEnvironment(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SAAS_ENV_FILE_VALUE") != "from-file" || os.Getenv("SAAS_ENV_OVERRIDE") != "from-process" {
		t.Fatal("module .env was not loaded or process override was overwritten")
	}
}

func TestLoadEnvironmentPrefersCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(filepath.Join(root, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]string{filepath.Join(root, ".env"): "current", filepath.Join(root, "server", ".env"): "nested"} {
		if err := os.WriteFile(path, []byte("SAAS_ENV_ORDER="+value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SAAS_ENV_ORDER", "")
	if err := os.Unsetenv("SAAS_ENV_ORDER"); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvironment(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SAAS_ENV_ORDER") != "current" {
		t.Fatal("the selected .env did not belong to the current directory")
	}
}

func TestLoadEnvironmentAllowsMissingFileAndRedactsMalformedValues(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := LoadEnvironment(); err != nil {
		t.Fatalf("environment-only deployments must not require .env: %v", err)
	}
	if err := os.WriteFile(".env", []byte("SAAS_ENV_BROKEN=\"sensitive-unclosed-value"), 0600); err != nil {
		t.Fatal(err)
	}
	err := LoadEnvironment()
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("malformed .env was ignored or sensitive value was exposed")
	}
}
