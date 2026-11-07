// Package httpclient 提供有界、脱敏且禁止跨服务重定向的外部 HTTP 访问。
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func New() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Minute,
		// 配置服务地址可指向本地网络；禁止跳转以免把授权头带到另一服务。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func Endpoint(endpoint, suffix string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("服务地址必须是有效的 HTTP(S) URL，不能包含凭据、查询或片段")
	}
	u.Path = strings.TrimRight(u.Path, "/") + suffix
	u.RawPath = ""
	return u.String(), nil
}

func JSON(ctx context.Context, client *http.Client, method, address string, headers map[string]string, input, output any, limit int64) (int, error) {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return 0, errors.New("无法编码服务请求")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return 0, errors.New("无法创建服务请求")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Timeout() {
			return 0, errors.New("外部服务请求超时")
		}
		// 上游错误可能回显 URL 或密钥，不返回原始 URLError。
		return 0, errors.New("外部服务连接失败")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return resp.StatusCode, errors.New("读取外部服务响应失败")
	}
	if len(data) > int(limit) {
		return resp.StatusCode, errors.New("外部服务响应超过大小限制")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("外部服务返回 HTTP %d", resp.StatusCode)
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			return resp.StatusCode, errors.New("外部服务返回无效 JSON")
		}
	}
	return resp.StatusCode, nil
}
