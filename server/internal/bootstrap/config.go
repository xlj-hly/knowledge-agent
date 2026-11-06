// Package bootstrap 读取部署配置并组装应用，不保存各企业的业务凭据。
package bootstrap

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go-server/internal/domain/saas"
	objectstorage "go-server/internal/infra/storage"
)

// Config 保存部署级参数；企业 AppID 与 AppSecret 由后台写入数据库。
type Config struct {
	DatabaseURL    string
	ListenAddr     string
	AllowedOrigins []string
	PlatformHosts  []string
	Storage        objectstorage.Config
	SessionTTL     time.Duration
}

// LoadConfig 读取明确的环境变量；错误仅携带字段用途，不回显配置值。
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{DatabaseURL: getenv("DATABASE_URL"), ListenAddr: getenv("LISTEN_ADDR")}
	cfg.Storage = objectstorage.Config{
		Provider: getenv("STORAGE_PROVIDER"), FilesystemRoot: getenv("STORAGE_FILESYSTEM_ROOT"),
		S3: objectstorage.S3Config{Endpoint: getenv("S3_ENDPOINT"), Region: getenv("S3_REGION"), Bucket: getenv("S3_BUCKET"), AccessKeyID: getenv("S3_ACCESS_KEY_ID"), SecretAccessKey: getenv("S3_SECRET_ACCESS_KEY")},
	}
	if value := getenv("S3_USE_PATH_STYLE"); value != "" {
		pathStyle, err := strconv.ParseBool(value)
		if err != nil {
			return cfg, fmt.Errorf("S3_USE_PATH_STYLE 必须是有效布尔值")
		}
		cfg.Storage.S3.UsePathStyle = pathStyle
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return cfg, fmt.Errorf("请设置 DATABASE_URL")
	}
	for _, origin := range strings.Split(getenv("ALLOWED_ORIGINS"), ",") {
		if value := strings.TrimSpace(origin); value != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, value)
		}
	}
	for _, host := range strings.Split(getenv("PLATFORM_HOSTS"), ",") {
		cfg.PlatformHosts = append(cfg.PlatformHosts, host)
	}
	return normalizeConfig(cfg)
}

// normalizeConfig 同时校验进程配置和测试显式注入的配置，不读取隐式全局状态。
func normalizeConfig(cfg Config) (Config, error) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 7 * 24 * time.Hour
	}
	if cfg.Storage.Provider == "" {
		cfg.Storage.Provider = "filesystem"
	}
	if cfg.Storage.FilesystemRoot == "" {
		cfg.Storage.FilesystemRoot = filepath.Join(".local", "objects")
	}
	if cfg.Storage.S3.Region == "" {
		cfg.Storage.S3.Region = "us-east-1"
	}
	if err := objectstorage.ValidateConfig(cfg.Storage); err != nil {
		return cfg, err
	}
	seen := make(map[string]bool)
	hosts := make([]string, 0, len(cfg.PlatformHosts))
	for _, entry := range cfg.PlatformHosts {
		host, err := domain.NormalizeHostname(entry)
		if err != nil {
			return cfg, fmt.Errorf("请设置有效的 PLATFORM_HOSTS 平台管理域名列表")
		}
		if !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		return cfg, fmt.Errorf("请设置有效的 PLATFORM_HOSTS 平台管理域名列表")
	}
	cfg.PlatformHosts = hosts
	return cfg, nil
}
