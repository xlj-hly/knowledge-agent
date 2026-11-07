package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/storage"
	domain "go-server/internal/domain/saas"
	"io"
	"mime"
	"net/http"
	"strconv"
)

// KnowledgeUseCases 是知识库 HTTP 入口实际消费的公开用例。
type KnowledgeUseCases interface {
	VectorProviders(context.Context, dto.Identity) ([]dto.VectorProvider, error)
	VectorStores(context.Context, dto.Identity) ([]dto.VectorStore, error)
	SaveVectorStore(context.Context, dto.Identity, string, dto.VectorStoreInput) (dto.VectorStore, error)
	DeleteVectorStore(context.Context, dto.Identity, string) (dto.OK, error)
	TestVectorStore(context.Context, dto.Identity, string) (dto.OK, error)
	Bases(context.Context, dto.Identity) ([]dto.Base, error)
	Base(context.Context, dto.Identity, string) (dto.Base, error)
	SaveBase(context.Context, dto.Identity, string, dto.BaseInput) (dto.Base, error)
	DeleteBase(context.Context, dto.Identity, string) (dto.OK, error)
	Rebuild(context.Context, dto.Identity, string, dto.BaseInput) (dto.Base, error)
	Preview(context.Context, dto.Identity, string, dto.PreviewInput) ([]dto.PreviewChunk, error)
	Documents(context.Context, dto.Identity, string) ([]dto.Document, error)
	CreateDocument(context.Context, dto.Identity, string, dto.DocumentInput) (dto.Document, error)
	UploadDocument(context.Context, dto.Identity, string, string, []byte) (dto.Document, error)
	Document(context.Context, dto.Identity, string) (dto.Document, error)
	UpdateDocument(context.Context, dto.Identity, string, dto.DocumentInput) (dto.Document, error)
	DeleteDocument(context.Context, dto.Identity, string) (dto.OK, error)
	Reprocess(context.Context, dto.Identity, string) (dto.Document, error)
	Chunks(context.Context, dto.Identity, string) ([]dto.Chunk, error)
	UpdateChunk(context.Context, dto.Identity, string, dto.ChunkInput) (dto.Document, error)
	Search(context.Context, dto.Identity, string, dto.SearchInput) ([]dto.Hit, error)
	Download(context.Context, dto.Identity, string) (storage.Object, string, error)
}

func registerKnowledgeRoutes(api *gin.RouterGroup, k KnowledgeUseCases) {
	if k == nil {
		return
	}
	r := api.Group("/knowledge")
	r.GET("/vector-providers", knowledgeRead(k.VectorProviders))
	r.GET("/vector-stores", knowledgeRead(k.VectorStores))
	r.POST("/vector-stores", vectorStoreInput(k.SaveVectorStore))
	r.PATCH("/vector-stores/:id", vectorStoreInput(k.SaveVectorStore))
	r.DELETE("/vector-stores/:id", knowledgeTarget(k.DeleteVectorStore))
	r.POST("/vector-stores/:id/test", knowledgeTarget(k.TestVectorStore))
	r.POST("/preview", knowledgeInput(k.Preview))
	r.POST("/bases/:id/rebuild", knowledgeInput(k.Rebuild))
	r.GET("/bases", knowledgeRead(k.Bases))
	r.POST("/bases", knowledgeInput(k.SaveBase))
	r.GET("/bases/:id", knowledgeTarget(k.Base))
	r.PATCH("/bases/:id", knowledgeInput(k.SaveBase))
	r.DELETE("/bases/:id", knowledgeTarget(k.DeleteBase))
	r.GET("/bases/:id/documents", knowledgeTarget(k.Documents))
	r.POST("/bases/:id/documents", knowledgeInput(k.CreateDocument))
	r.POST("/bases/:id/upload", knowledgeUpload(k))
	r.POST("/bases/:id/search", knowledgeInput(k.Search))
	r.GET("/documents/:id", knowledgeTarget(k.Document))
	r.PATCH("/documents/:id", knowledgeInput(k.UpdateDocument))
	r.DELETE("/documents/:id", knowledgeTarget(k.DeleteDocument))
	r.POST("/documents/:id/reprocess", knowledgeTarget(k.Reprocess))
	r.GET("/documents/:id/chunks", knowledgeTarget(k.Chunks))
	r.GET("/documents/:id/download", knowledgeDownload(k))
	r.PATCH("/chunks/:id", knowledgeInput(k.UpdateChunk))
}

// decodeVectorStore 在传输边界保留 map 值的 null 与空字符串区别。
func decodeVectorStore(c *gin.Context) (dto.VectorStoreInput, error) {
	var wire struct {
		Name     string                     `json:"name"`
		Provider string                     `json:"provider"`
		Config   map[string]json.RawMessage `json:"config"`
	}
	if err := decodeObject(c, &wire, 64*1024); err != nil {
		return dto.VectorStoreInput{}, err
	}
	if wire.Config == nil {
		return dto.VectorStoreInput{}, domain.Invalid("向量配置必须是字符串对象")
	}
	config := make(map[string]string, len(wire.Config))
	for key, raw := range wire.Config {
		var value *string
		if err := json.Unmarshal(raw, &value); err != nil || value == nil {
			return dto.VectorStoreInput{}, domain.Invalid("向量配置字段必须是字符串")
		}
		config[key] = *value
	}
	return dto.VectorStoreInput{Name: wire.Name, Provider: wire.Provider, Config: config}, nil
}

func vectorStoreInput(fn func(context.Context, dto.Identity, string, dto.VectorStoreInput) (dto.VectorStore, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		input, err := decodeVectorStore(c)
		if err != nil {
			return nil, err
		}
		id, err := knowledgeIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, c.Param("id"), input)
	})
}
func knowledgeIdentity(c *gin.Context) (dto.Identity, error) {
	id, err := identity(c)
	return dto.Identity{Hostname: id.Hostname, Token: id.Token}, err
}
func knowledgeRead[O any](fn func(context.Context, dto.Identity) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := knowledgeIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id)
	})
}
func knowledgeTarget[O any](fn func(context.Context, dto.Identity, string) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := knowledgeIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, c.Param("id"))
	})
}
func knowledgeInput[I, O any](fn func(context.Context, dto.Identity, string, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decodeKnowledge(c, &in); err != nil {
			return nil, err
		}
		id, err := knowledgeIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, c.Param("id"), in)
	})
}
func decodeKnowledge(c *gin.Context, out any) error {
	return decodeObject(c, out, 2*1024*1024+65536)
}
func knowledgeUpload(k KnowledgeUseCases) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := knowledgeIdentity(c)
		if err != nil {
			return nil, err
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10*1024*1024+65536)
		if err = c.Request.ParseMultipartForm(1024 * 1024); err != nil {
			return nil, domain.Invalid("文件无效或超过 10 MiB")
		}
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		files := c.Request.MultipartForm.File["file"]
		if len(files) != 1 || len(c.Request.MultipartForm.File) != 1 {
			return nil, domain.Invalid("请选择一个文件")
		}
		f, err := files[0].Open()
		if err != nil {
			return nil, domain.Invalid("无法读取文件")
		}
		defer f.Close()
		body, err := io.ReadAll(io.LimitReader(f, 10*1024*1024+1))
		if err != nil {
			return nil, domain.Invalid("无法读取文件")
		}
		return k.UploadDocument(c.Request.Context(), id, c.Param("id"), files[0].Filename, body)
	})
}
func knowledgeDownload(k KnowledgeUseCases) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := knowledgeIdentity(c)
		if err != nil {
			reply(c, nil, err)
			return
		}
		obj, name, err := k.Download(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			reply(c, nil, err)
			return
		}
		defer obj.Body.Close()
		c.Header("Content-Type", obj.ContentType)
		c.Header("Content-Length", strconv.FormatInt(obj.Size, 10))
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		c.Header("Cache-Control", "private, no-store")
		c.Status(http.StatusOK)
		if _, err = io.Copy(c.Writer, obj.Body); err != nil {
			logFailure("knowledge download", err)
		}
	}
}
