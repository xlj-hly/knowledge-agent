package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go-server/internal/app/chat/dto"
	domain "go-server/internal/domain/saas"
)

type streamChat struct {
	started bool
	failure error
	calls   int
}

func (*streamChat) Sessions(context.Context, dto.Identity) ([]dto.Session, error) {
	return []dto.Session{}, nil
}
func (*streamChat) Session(context.Context, dto.Identity, string) (dto.Session, error) {
	return dto.Session{}, nil
}
func (*streamChat) SaveSession(context.Context, dto.Identity, string, dto.SessionInput) (dto.Session, error) {
	return dto.Session{}, nil
}
func (*streamChat) DeleteSession(context.Context, dto.Identity, string) (dto.OK, error) {
	return dto.OK{OK: true}, nil
}
func (*streamChat) Messages(context.Context, dto.Identity, string) ([]dto.Message, error) {
	return []dto.Message{}, nil
}
func (*streamChat) Stop(context.Context, dto.Identity, string) (dto.OK, error) {
	return dto.OK{OK: true}, nil
}
func (s *streamChat) Send(_ context.Context, id dto.Identity, _ string, _ dto.MessageInput, emit func(dto.Event) error) error {
	s.calls++
	if id.Hostname != "alpha.localhost" || id.Token != "session-token" {
		return errors.New("incorrect identity")
	}
	if s.started {
		if err := emit(dto.Event{Type: "message", Message: dto.Message{ID: "1", Role: "assistant", Status: "generating", Sources: []dto.Source{}}}); err != nil {
			return err
		}
		if err := emit(dto.Event{Type: "delta", Delta: dto.Delta{Content: "正文\n下一行"}}); err != nil {
			return err
		}
	}
	return s.failure
}

func TestChatStreamKeepsJSONErrorsBeforeStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &streamChat{failure: domain.Forbidden()}
	router := gin.New()
	registerChatRoutes(router.Group("/api"), service)
	req := httptest.NewRequest("POST", "http://alpha.localhost/api/chat/sessions/1/messages", strings.NewReader(`{"content":"问题"}`))
	req.Header.Set("Authorization", "Bearer session-token")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	if result.Code != 403 || !strings.Contains(result.Body.String(), `"code":"forbidden"`) {
		t.Fatalf("incorrect pre-stream failure: %d %s", result.Code, result.Body.String())
	}
}

func TestChatStreamEncodesDeltasAndSanitizesLateFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &streamChat{started: true, failure: errors.New("secret-upstream-key")}
	router := gin.New()
	registerChatRoutes(router.Group("/api"), service)
	req := httptest.NewRequest("POST", "http://alpha.localhost/api/chat/sessions/1/messages", strings.NewReader(`{"content":"问题"}`))
	req.Header.Set("Authorization", "Bearer session-token")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	body := result.Body.String()
	if result.Code != 200 || !strings.Contains(result.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream headers: %d %v", result.Code, result.Header())
	}
	if !strings.Contains(body, "event: message\n") || !strings.Contains(body, "event: delta\n") || !strings.Contains(body, "event: error\n") || !strings.Contains(body, `正文\n下一行`) {
		t.Fatalf("invalid SSE: %s", body)
	}
	if strings.Contains(body, "secret-upstream-key") {
		t.Fatal("raw upstream error leaked")
	}
	if result.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatal("proxy buffering was not disabled")
	}
}

func TestChatRejectsNullMessageBeforeUsecase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &streamChat{}
	router := gin.New()
	registerChatRoutes(router.Group("/api"), service)
	req := httptest.NewRequest("POST", "http://alpha.localhost/api/chat/sessions/1/messages", strings.NewReader(`{"content":null}`))
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	if result.Code != 400 || service.calls != 0 {
		t.Fatalf("invalid null reached usecase: %d calls=%d", result.Code, service.calls)
	}
}
