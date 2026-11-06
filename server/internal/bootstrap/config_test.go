package bootstrap

import (
	"path/filepath"
	"strings"
	"testing"
)

func configEnvironment(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"DATABASE_URL":            "postgres://local:example@127.0.0.1:5432/test",
		"PLATFORM_HOSTS":          "Platform.Localhost,admin.localhost,platform.localhost",
		"STORAGE_FILESYSTEM_ROOT": filepath.Join(t.TempDir(), "objects"),
	}
}

func TestLoadConfigKeepsTenantCredentialsOutOfDeploymentConfiguration(t *testing.T) {
	env := configEnvironment(t)
	env["WECHAT_MINIPROGRAMS_FILE"] = "missing-obsolete-file.json"
	cfg, err := LoadConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.PlatformHosts, ",") != "platform.localhost,admin.localhost" {
		t.Fatal("platform hosts were not loaded correctly")
	}
	if cfg.Storage.FilesystemRoot != env["STORAGE_FILESYSTEM_ROOT"] {
		t.Fatal("persistent object location was not loaded")
	}
}

func TestLoadConfigRejectsUnusableConfigurationWithoutExposingValues(t *testing.T) {
	for name, mutate := range map[string]func(map[string]string){
		"missing database URL":   func(env map[string]string) { env["DATABASE_URL"] = "" },
		"missing platform hosts": func(env map[string]string) { env["PLATFORM_HOSTS"] = "" },
		"invalid platform host":  func(env map[string]string) { env["PLATFORM_HOSTS"] = "https://secret-invalid-host" },
	} {
		t.Run(name, func(t *testing.T) {
			env := configEnvironment(t)
			mutate(env)
			_, err := LoadConfig(func(key string) string { return env[key] })
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("unusable deployment configuration accepted or sensitive value exposed")
			}
		})
	}
}

func TestLoadConfigOwnsRuntimeAndHTTPConfiguration(t *testing.T) {
	env := configEnvironment(t)
	env["LISTEN_ADDR"] = "127.0.0.1:9000"
	env["ALLOWED_ORIGINS"] = " https://web.example.com, ,https://admin.example.com "
	cfg, err := LoadConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != env["DATABASE_URL"] || cfg.ListenAddr != "127.0.0.1:9000" || strings.Join(cfg.AllowedOrigins, ",") != "https://web.example.com,https://admin.example.com" {
		t.Fatal("application configuration is split across unrelated entry points")
	}
}

func TestLoadConfigDefaultsStorageToPrivateLocalDirectory(t *testing.T) {
	env := configEnvironment(t)
	delete(env, "STORAGE_FILESYSTEM_ROOT")
	cfg, err := LoadConfig(func(key string) string { return env[key] })
	if err != nil || cfg.Storage.Provider != "filesystem" || cfg.Storage.FilesystemRoot != filepath.Join(".local", "objects") {
		t.Fatal("default object directory is not the private application directory")
	}
}

func TestLoadConfigDoesNotRequireWeChatEncryptionKey(t *testing.T) {
	env := configEnvironment(t)
	delete(env, "WECHAT_ENCRYPTION_KEY")
	if _, err := LoadConfig(func(key string) string { return env[key] }); err != nil {
		t.Fatalf("company credentials do not require a deployment encryption key: %v", err)
	}
}
