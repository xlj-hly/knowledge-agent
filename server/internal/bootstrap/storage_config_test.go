package bootstrap

import (
	"strings"
	"testing"
)

func TestLoadConfigSelectsS3WithoutTenantCredentials(t *testing.T) {
	env := configEnvironment(t)
	env["STORAGE_PROVIDER"] = "s3"
	env["S3_ENDPOINT"] = "http://127.0.0.1:9002"
	env["S3_REGION"] = "us-east-1"
	env["S3_BUCKET"] = "agent"
	env["S3_ACCESS_KEY_ID"] = "test-access-key"
	env["S3_SECRET_ACCESS_KEY"] = "test-secret"
	env["S3_USE_PATH_STYLE"] = "true"
	env["WECHAT_MINIPROGRAMS_FILE"] = "missing-obsolete-file.json"
	cfg, err := LoadConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Provider != "s3" || cfg.Storage.S3.Endpoint != env["S3_ENDPOINT"] || cfg.Storage.S3.Bucket != "agent" || cfg.Storage.S3.AccessKeyID != env["S3_ACCESS_KEY_ID"] || cfg.Storage.S3.SecretAccessKey != env["S3_SECRET_ACCESS_KEY"] || !cfg.Storage.S3.UsePathStyle {
		t.Fatal("S3配置未完整传入适配器配置")
	}
}

func TestLoadConfigRejectsInvalidStorageConfigurationWithoutExposingValues(t *testing.T) {
	for name, mutate := range map[string]func(map[string]string){
		"invalid provider":   func(env map[string]string) { env["STORAGE_PROVIDER"] = "secret-unknown-provider" },
		"invalid path style": func(env map[string]string) { env["S3_USE_PATH_STYLE"] = "secret-invalid-bool" },
	} {
		t.Run(name, func(t *testing.T) {
			env := configEnvironment(t)
			mutate(env)
			_, err := LoadConfig(func(key string) string { return env[key] })
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("无效配置被接受或错误泄漏原值")
			}
		})
	}
}
