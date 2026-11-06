package domain

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_.-]{3,64}$`)
var hostnameLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var avatarPath = regexp.MustCompile(`^/api/avatars/[1-9][0-9]{0,18}/[1-9][0-9]{0,18}/[0-9a-f]{32}\.(png|jpg)$`)

// ParseID 解析正整数形式的 BIGINT ID，拒绝零、负数和越界值。
func ParseID(input string) (int64, error) {
	id, err := strconv.ParseInt(input, 10, 64)
	if err != nil || id <= 0 {
		return 0, Invalid("无效 ID")
	}
	return id, nil
}

// NormalizeHostname 将合法域名规范为小写并去除有效端口，拒绝协议、路径和空标签。
func NormalizeHostname(input string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(input))
	if strings.Contains(host, ":") {
		h, p, err := net.SplitHostPort(host)
		port, portErr := strconv.Atoi(p)
		if err != nil || portErr != nil || port < 1 || port > 65535 {
			return "", Invalid("请输入有效域名（不含协议和路径）")
		}
		host = h
	}
	if len(host) == 0 || len(host) > 253 {
		return "", Invalid("请输入有效域名")
	}
	for _, label := range strings.Split(host, ".") {
		if !hostnameLabel.MatchString(label) {
			return "", Invalid("请输入有效域名（不含协议和路径）")
		}
	}
	return host, nil
}

// NormalizeUsername 规范化用户名并检查 ASCII 字符和长度，不允许以路径或中文作为登录名。
func NormalizeUsername(input string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(input))
	if !usernamePattern.MatchString(name) {
		return "", Invalid("用户名需为 3–64 位字母、数字、下划线、点或横线")
	}
	return name, nil
}

// ValidatePassword 按 UTF-8 字节检查 10–72 字节范围，与 bcrypt 输入限制保持一致。
func ValidatePassword(input string) error {
	if !utf8.ValidString(input) || len(input) < 10 || len(input) > 72 {
		return Invalid("密码需为 10–72 字节")
	}
	return nil
}

// ValidateAvatar 允许空值、受控头像相对路径或无凭据HTTP(S)地址；文件归属由用例校验。
func ValidateAvatar(input string) error {
	if input == "" {
		return nil
	}
	if avatarPath.MatchString(input) {
		return nil
	}
	u, err := url.Parse(input)
	if err != nil || len(input) > 2048 || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return Invalid("头像需为有效 HTTP(S) 地址")
	}
	return nil
}

// ValidateName 校验非空名称和 Unicode 字符数；label 只用于构造可公开的错误提示。
func ValidateName(input string, max int, label string) error {
	if !utf8.ValidString(input) || strings.TrimSpace(input) == "" || utf8.RuneCountInString(input) > max {
		return Invalid(label + "不能为空且长度超限")
	}
	return nil
}
