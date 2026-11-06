package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

type cleanupDiagnostic struct{}

func (cleanupDiagnostic) Error() string       { return "secret-access-key-and-object-details" }
func (cleanupDiagnostic) Operation() string   { return "delete uncommitted avatar" }
func (cleanupDiagnostic) FailureCode() string { return "AccessDenied" }

func TestBusinessRejectionStillReportsCleanupFailureWithoutSecrets(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	deps := injectedDependencies()
	deps.Auth = injectedAuth{login: func(context.Context, dto.TenantIdentity, dto.LoginInput) (dto.TenantSession, error) {
		return dto.TenantSession{}, errors.Join(domain.Unauthorized(), cleanupDiagnostic{})
	}}
	req := httptest.NewRequest(http.MethodPost, "http://alpha.test/api/auth/login", strings.NewReader(`{"username":"member","password":"password-value"}`))
	w := httptest.NewRecorder()
	New(deps, Config{}).ServeHTTP(w, req)
	if w.Code != 401 || !strings.Contains(w.Body.String(), `"code":"unauthorized"`) {
		t.Fatalf("认证失败契约改变：%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(output.String(), "delete uncommitted avatar") || !strings.Contains(output.String(), `"error_code":"AccessDenied"`) || strings.Contains(output.String(), "secret-access") || strings.Contains(w.Body.String(), "secret-access") {
		t.Fatalf("清理故障未脱敏记录：%s", output.String())
	}
}
