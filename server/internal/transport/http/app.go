// Package httpapi 将 SaaS 用例适配为公开 Gin HTTP 契约。
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

// Config 保存 HTTP 适配层的跨域来源配置。
type Config struct {
	AllowedOrigins []string
}

// New 使用已注入的业务用例组装路由，不拥有其依赖生命周期。
func New(deps Dependencies, cfg Config) http.Handler {
	r := gin.New()
	r.Use(recoverRequest(), security(), cors(cfg.AllowedOrigins), platformEntry(deps.Sites))
	registerSiteRoutes(r, deps.Sites, deps.Maintenance)
	api := r.Group("/api")
	registerAuthRoutes(api, deps.Auth)
	registerAccountRoutes(api, deps.Accounts, deps.Auth)
	registerCompanyRoutes(api, deps.Companies)
	registerAIRoutes(api, deps.AI)
	registerChatRoutes(api, deps.Chat)
	if deps.Knowledge != nil {
		registerKnowledgeRoutes(api, deps.Knowledge)
	}
	return r
}
func endpoint(fn func(*gin.Context) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) { data, err := fn(c); reply(c, data, err) }
}
func input[I, O any](fn func(context.Context, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decode(c, &in); err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), in)
	})
}
func tenantRead[O any](fn func(context.Context, dto.TenantIdentity) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := identity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id)
	})
}
func tenantInput[I, O any](fn func(context.Context, dto.TenantIdentity, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decode(c, &in); err != nil {
			return nil, err
		}
		id, err := identity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, in)
	})
}
func tenantTarget[I, O any](fn func(context.Context, dto.TenantIdentity, string, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decode(c, &in); err != nil {
			return nil, err
		}
		id, err := identity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, c.Param("id"), in)
	})
}
func platformRead[O any](fn func(context.Context, string) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		token, err := bearer(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), token)
	})
}
func platformInput[I, O any](fn func(context.Context, string, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decode(c, &in); err != nil {
			return nil, err
		}
		token, err := bearer(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), token, in)
	})
}
func platformTarget[I, O any](fn func(context.Context, string, string, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decode(c, &in); err != nil {
			return nil, err
		}
		token, err := bearer(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), token, c.Param("id"), in)
	})
}

func reply(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"data": data})
		return
	}
	var business *domain.Error
	status, code, message := http.StatusInternalServerError, "internal_error", "服务暂时不可用，请稍后再试"
	if errors.As(err, &business) {
		code, message = business.Code, business.Message
		switch code {
		case "invalid_input", "invalid_password":
			status = 400
		case "unauthorized", "invalid_credentials":
			status = 401
		case "wechat_invalid_code":
			status = 401
		case "wechat_not_configured", "wechat_unavailable":
			status = 503
		case "wechat_upstream_error":
			status = 502
		case "knowledge_upstream_error", "ai_upstream_error", "chat_upstream_error":
			status = 502
		case "wechat_rate_limited":
			status = 429
			c.Header("Retry-After", "60")
		case "forbidden", "disabled", "wechat_login_blocked":
			status = 403
		case "not_found":
			status = 404
		case "conflict":
			status = 409
		}
	}
	// 业务拒绝仍可能伴随资源回收失败；响应保留业务语义，基础设施故障必须可诊断。
	var infrastructure interface{ Operation() string }
	if status == 500 || errors.As(err, &infrastructure) {
		logFailure(c.FullPath(), err)
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
func logFailure(operation string, err error) {
	// 仅保留操作及适配器提供的脱敏类别，禁止输出带SQL、对象地址或凭据的原始错误。
	var state interface{ SQLState() string }
	sqlState := ""
	if errors.As(err, &state) {
		sqlState = state.SQLState()
	}
	var source interface{ Operation() string }
	if errors.As(err, &source) {
		operation += ": " + source.Operation()
	}
	var classified interface{ FailureCode() string }
	errorCode := ""
	if errors.As(err, &classified) {
		errorCode = classified.FailureCode()
	}
	cause := "infrastructure"
	if errors.Is(err, context.DeadlineExceeded) {
		cause = "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		cause = "canceled"
	}
	slog.Error("SaaS request failed", "operation", operation, "cause", cause, "sqlstate", sqlState, "error_code", errorCode)
}
