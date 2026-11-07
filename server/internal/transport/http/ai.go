package httpapi

import (
	"context"
	"github.com/gin-gonic/gin"
	"go-server/internal/app/ai/dto"
)

// AIUseCases 是已授权配置入口使用的模型与默认模板能力。
type AIUseCases interface {
	Providers(context.Context, dto.Identity) ([]dto.Provider, error)
	Models(context.Context, dto.Identity) ([]dto.Model, error)
	SaveModel(context.Context, dto.Identity, string, dto.ModelInput) (dto.Model, error)
	DeleteModel(context.Context, dto.Identity, string) (dto.OK, error)
	ActivateModel(context.Context, dto.Identity, string) (dto.Settings, error)
	Discover(context.Context, dto.Identity, dto.ModelInput) (dto.Discovery, error)
	Test(context.Context, dto.Identity, dto.ModelInput) (dto.TestResult, error)
	Settings(context.Context, dto.Identity) (dto.Settings, error)
	SaveSettings(context.Context, dto.Identity, dto.SettingsInput) (dto.Settings, error)
	Presets(context.Context, dto.Identity) (dto.Presets, error)
}

func registerAIRoutes(api *gin.RouterGroup, s AIUseCases) {
	if s == nil {
		return
	}
	r := api.Group("/ai")
	r.GET("/providers", aiRead(s.Providers))
	r.GET("/models", aiRead(s.Models))
	r.GET("/settings", aiRead(s.Settings))
	r.GET("/presets", aiRead(s.Presets))
	r.POST("/models/discover", aiInput(s.Discover))
	r.POST("/models/test", aiInput(s.Test))
	r.PUT("/settings", aiInput(s.SaveSettings))
	r.POST("/models", aiTarget(s.SaveModel))
	r.PUT("/models/:id", aiTarget(s.SaveModel))
	r.PUT("/models/:id/activate", endpoint(func(c *gin.Context) (any, error) {
		id, err := aiIdentity(c)
		if err != nil {
			return nil, err
		}
		return s.ActivateModel(c.Request.Context(), id, c.Param("id"))
	}))
	r.DELETE("/models/:id", endpoint(func(c *gin.Context) (any, error) {
		id, err := aiIdentity(c)
		if err != nil {
			return nil, err
		}
		return s.DeleteModel(c.Request.Context(), id, c.Param("id"))
	}))
}
func aiIdentity(c *gin.Context) (dto.Identity, error) {
	id, err := identity(c)
	return dto.Identity{Hostname: id.Hostname, Token: id.Token}, err
}
func aiRead[O any](fn func(context.Context, dto.Identity) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := aiIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id)
	})
}
func aiInput[I, O any](fn func(context.Context, dto.Identity, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decodeObject(c, &in, 64*1024); err != nil {
			return nil, err
		}
		id, err := aiIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, in)
	})
}
func aiTarget[I, O any](fn func(context.Context, dto.Identity, string, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decodeObject(c, &in, 64*1024); err != nil {
			return nil, err
		}
		id, err := aiIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, c.Param("id"), in)
	})
}
