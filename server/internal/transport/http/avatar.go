package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go-server/internal/app/saas/dto"
	"go-server/internal/app/storage"
	"go-server/internal/domain/saas"
)

type tenantAuthenticator interface {
	Me(context.Context, dto.TenantIdentity) (dto.TenantSession, error)
}

func avatarUpload(s AccountUseCases, auth tenantAuthenticator) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := identity(c)
		if err != nil {
			return nil, err
		}
		// 在读取上传体前认证；文件内容在开启写事务和持有业务锁前完成有界读取。
		if _, err := auth.Me(c.Request.Context(), id); err != nil {
			return nil, err
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, domain.AvatarMaxBytes+64*1024)
		reader, err := c.Request.MultipartReader()
		if err != nil {
			return nil, domain.Invalid("请上传PNG或JPEG头像文件")
		}
		part, err := reader.NextPart()
		if err != nil || part.FormName() != "file" || part.FileName() == "" {
			return nil, domain.Invalid("头像文件字段必须为file")
		}
		defer part.Close()
		body, err := io.ReadAll(io.LimitReader(part, domain.AvatarMaxBytes+1))
		if err != nil || len(body) > domain.AvatarMaxBytes {
			return nil, domain.Invalid("头像不能超过2MiB")
		}
		if _, err := reader.NextPart(); err != io.EOF {
			return nil, domain.Invalid("一次只能上传一个头像文件")
		}
		return s.UploadAvatar(c.Request.Context(), id, body)
	})
}
func avatarRead(s AccountUseCases) gin.HandlerFunc {
	return func(c *gin.Context) {
		object, err := s.OpenAvatar(c.Request.Context(), c.Param("tenant"), c.Param("user"), c.Param("name"))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				reply(c, nil, domain.NotFound())
			} else {
				reply(c, nil, err)
			}
			return
		}
		// 头像体积有上限；先完整读取可在响应前发现远程读取错误，同时保留 Range/HEAD 契约。
		body, readErr := io.ReadAll(io.LimitReader(object.Body, domain.AvatarMaxBytes+1))
		closeErr := object.Body.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			reply(c, nil, err)
			return
		}
		if int64(len(body)) != object.Size || len(body) > domain.AvatarMaxBytes {
			reply(c, nil, fmt.Errorf("头像对象长度无效"))
			return
		}
		c.Header("Content-Type", object.ContentType)
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("X-Content-Type-Options", "nosniff")
		http.ServeContent(c.Writer, c.Request, c.Param("name"), object.ModifiedAt, bytes.NewReader(body))
	}
}
