// Package storage 将显式配置适配为供应商无关的对象存储端口。
package storage

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	appstorage "go-server/internal/app/storage"
)

// Config 由装配层提供，不隐式读取环境变量或全局配置。
type Config struct {
	Provider       string
	FilesystemRoot string
	S3             S3Config
}

// S3Config 同时支持标准 S3 endpoint 和 MinIO 等兼容服务。
type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
}

// New 创建对象存储，不自动创建 bucket 或进行远程探测。
func New(config Config) (appstorage.Store, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	switch config.Provider {
	case "filesystem":
		root, err := filepath.Abs(config.FilesystemRoot)
		if err != nil {
			return nil, failed("filesystem", "configure", err)
		}
		return &filesystemStore{root: root}, nil
	case "s3":
		return newS3(config.S3), nil
	default:
		return nil, errors.New("unsupported object storage provider")
	}
}

// ValidateConfig 校验配置的完整性与安全边界，不读取文件或访问远程服务。
func ValidateConfig(config Config) error {
	switch config.Provider {
	case "filesystem":
		if strings.TrimSpace(config.FilesystemRoot) == "" || strings.ContainsRune(config.FilesystemRoot, 0) {
			return errors.New("filesystem object storage root is required")
		}
	case "s3":
		value := config.S3
		if strings.TrimSpace(value.Region) == "" || strings.TrimSpace(value.Bucket) == "" || strings.TrimSpace(value.AccessKeyID) == "" || strings.TrimSpace(value.SecretAccessKey) == "" {
			return errors.New("s3 object storage region, bucket and credentials are required")
		}
		if strings.ContainsAny(value.Bucket, `/\\:?*`) || strings.ContainsFunc(value.Bucket, unicode.IsSpace) || strings.ContainsFunc(value.Bucket, unicode.IsControl) || strings.ContainsFunc(value.Region, unicode.IsSpace) || strings.ContainsFunc(value.Region, unicode.IsControl) {
			return errors.New("invalid s3 object storage region or bucket")
		}
		if value.Endpoint != "" {
			endpoint, err := url.Parse(value.Endpoint)
			if err != nil || endpoint.Scheme != "http" && endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.Path != "" && endpoint.Path != "/" {
				return errors.New("invalid s3 object storage endpoint")
			}
		}
	default:
		return errors.New("unsupported object storage provider")
	}
	return nil
}
