package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go-server/internal/app/chat/dto"
	domain "go-server/internal/domain/saas"
)

// ChatUseCases 是 HTTP 入口消费的会话和生成契约。
type ChatUseCases interface {
	Sessions(context.Context, dto.Identity) ([]dto.Session, error)
	Session(context.Context, dto.Identity, string) (dto.Session, error)
	SaveSession(context.Context, dto.Identity, string, dto.SessionInput) (dto.Session, error)
	DeleteSession(context.Context, dto.Identity, string) (dto.OK, error)
	Messages(context.Context, dto.Identity, string) ([]dto.Message, error)
	Send(context.Context, dto.Identity, string, dto.MessageInput, func(dto.Event) error) error
	Stop(context.Context, dto.Identity, string) (dto.OK, error)
}

func registerChatRoutes(api *gin.RouterGroup, chat ChatUseCases) {
	if chat == nil {
		return
	}
	r := api.Group("/chat")
	r.GET("/sessions", chatRead(chat.Sessions))
	r.POST("/sessions", chatInput(chat.SaveSession))
	r.GET("/sessions/:id", chatTarget(chat.Session))
	r.PUT("/sessions/:id", chatInput(chat.SaveSession))
	r.DELETE("/sessions/:id", chatTarget(chat.DeleteSession))
	r.GET("/sessions/:id/messages", chatTarget(chat.Messages))
	r.POST("/sessions/:id/messages", chatStream(chat))
	r.POST("/sessions/:id/stop", chatTarget(chat.Stop))
}
func chatIdentity(c *gin.Context) (dto.Identity, error) {
	id, err := identity(c)
	return dto.Identity{Hostname: id.Hostname, Token: id.Token}, err
}
func chatRead[O any](fn func(context.Context, dto.Identity) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := chatIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id)
	})
}
func chatTarget[O any](fn func(context.Context, dto.Identity, string) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		id, err := chatIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, c.Param("id"))
	})
}
func chatInput[I, O any](fn func(context.Context, dto.Identity, string, I) (O, error)) gin.HandlerFunc {
	return endpoint(func(c *gin.Context) (any, error) {
		var in I
		if err := decodeKnowledge(c, &in); err != nil {
			return nil, err
		}
		id, err := chatIdentity(c)
		if err != nil {
			return nil, err
		}
		return fn(c.Request.Context(), id, c.Param("id"), in)
	})
}

func chatStream(chat ChatUseCases) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in dto.MessageInput
		if err := decodeKnowledge(c, &in); err != nil {
			reply(c, nil, err)
			return
		}
		id, err := chatIdentity(c)
		if err != nil {
			reply(c, nil, err)
			return
		}
		started := false
		emit := func(event dto.Event) error {
			var data any
			switch event.Type {
			case "message", "done":
				data = event.Message
			case "sources":
				data = event.Sources
			case "delta":
				data = event.Delta
			case "error":
				data = event.Failure
			default:
				return errors.New("unknown chat stream event")
			}
			encoded, e := json.Marshal(data)
			if e != nil {
				return e
			}
			if !started {
				c.Header("Content-Type", "text/event-stream; charset=utf-8")
				c.Header("Cache-Control", "private, no-store")
				c.Header("X-Accel-Buffering", "no")
				c.Status(http.StatusOK)
				started = true
			}
			if _, e = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, encoded); e != nil {
				return e
			}
			c.Writer.Flush()
			return nil
		}
		err = chat.Send(c.Request.Context(), id, c.Param("id"), in, emit)
		if err == nil {
			return
		}
		if !started {
			reply(c, nil, err)
			return
		}
		if c.Request.Context().Err() != nil {
			return
		}
		failure := dto.Failure{Code: "internal_error", Message: "服务暂时不可用，请重新加载确认回答状态"}
		var business *domain.Error
		if errors.As(err, &business) {
			failure = dto.Failure{Code: business.Code, Message: business.Message}
		}
		logFailure("chat stream", err)
		if e := emit(dto.Event{Type: "error", Failure: failure}); e != nil {
			logFailure("chat stream delivery", e)
		}
	}
}
