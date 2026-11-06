package domain

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
)

// AvatarMaxBytes 限制单张头像原始文件为 2MiB，传输封装开销由 HTTP 层另行限制。
const AvatarMaxBytes = 2 * 1024 * 1024

// ValidateAvatarImage 完整解码 PNG/JPEG，限制边长与像素以避免压缩图像耗尽资源。
func ValidateAvatarImage(body []byte) (string, error) {
	if len(body) == 0 || len(body) > AvatarMaxBytes {
		return "", Invalid("头像图片无效：文件应为1字节至2MiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || format != "png" && format != "jpeg" || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 16*1024*1024 {
		return "", Invalid("头像图片无效：仅支持PNG或JPEG，边长不超过4096像素")
	}
	if _, _, err := image.Decode(bytes.NewReader(body)); err != nil {
		return "", Invalid("头像图片无效：文件内容不完整")
	}
	return format, nil
}
