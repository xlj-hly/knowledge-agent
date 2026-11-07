package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

// decode 在进入用例前完成有界读取和对象、字段 null 检查，避免绑定器把 null 当作零值。
func decode(c *gin.Context, out any) error {
	return decodeObject(c, out, 16*1024)
}

func decodeObject(c *gin.Context, out any, limit int64) error {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	if err != nil {
		return domain.Invalid("请求内容无效或超过大小限制")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return domain.Invalid("请求内容必须为单个 JSON 对象")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return domain.Invalid("请求内容必须为单个 JSON 对象")
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return domain.Invalid("字段不允许为 null")
		}
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return domain.Invalid("请求内容无效或包含不允许的字段")
	}
	return nil
}
func bearer(c *gin.Context) (string, error) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", nil
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 256 {
		return "", domain.Unauthorized()
	}
	return parts[1], nil
}
func identity(c *gin.Context) (dto.TenantIdentity, error) {
	token, err := bearer(c)
	return dto.TenantIdentity{Hostname: c.Request.Host, Token: token}, err
}
func pageQuery(c *gin.Context) (dto.Pagination, error) {
	p := dto.Pagination{Page: 1, Size: 20, Query: strings.TrimSpace(c.Query("q"))}
	var err error
	if v := c.Query("page"); v != "" {
		p.Page, err = strconv.Atoi(v)
		if err != nil {
			return p, domain.Invalid("分页参数无效")
		}
	}
	if v := c.Query("page_size"); v != "" {
		p.Size, err = strconv.Atoi(v)
		if err != nil {
			return p, domain.Invalid("分页参数无效")
		}
	}
	return p, p.Validate()
}
