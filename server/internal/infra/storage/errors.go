package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aws/smithy-go"

	appstorage "go-server/internal/app/storage"
)

type operationError struct {
	provider, operation string
	cause               error
}

func (e *operationError) Error() string {
	return "object storage " + e.provider + " " + e.operation + " failed"
}
func (e *operationError) Unwrap() error { return e.cause }

// Operation 为日志提供脱敏上下文，底层错误仍可用 errors.Is/As 判断。
func (e *operationError) Operation() string {
	return "object storage " + e.provider + " " + e.operation
}

// FailureCode 仅公开受控诊断类别，禁止把供应商错误文本或未知错误码写入日志。
// 原始错误链由 Unwrap 保留，取消和超时仍通过 errors.Is 判断。
func (e *operationError) FailureCode() string {
	if e.provider == "s3" {
		var apiError smithy.APIError
		if errors.As(e.cause, &apiError) {
			switch code := apiError.ErrorCode(); code {
			case "AccessDenied", "SignatureDoesNotMatch", "InvalidAccessKeyId", "NoSuchBucket", "RequestTimeout", "SlowDown", "ServiceUnavailable", "InternalError":
				return code
			}
		}
	}
	return "upstream_error"
}

func failed(provider, operation string, err error) error {
	return &operationError{provider: provider, operation: operation, cause: err}
}

func validateKey(key string) error {
	if key == "" || len(key) > 1024 || !utf8.ValidString(key) || strings.ContainsAny(key, `\:"<>|?*`) || strings.ContainsFunc(key, unicode.IsControl) {
		return appstorage.ErrInvalidKey
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return appstorage.ErrInvalidKey
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return appstorage.ErrInvalidKey
		}
	}
	return nil
}

func checkRequest(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return validateKey(key)
}

func contentTypeOrDefault(contentType string) (string, error) {
	if len(contentType) > 1024 || strings.ContainsFunc(contentType, unicode.IsControl) {
		return "", errors.New("invalid object content type")
	}
	if contentType == "" {
		return "application/octet-stream", nil
	}
	return contentType, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(body []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(body)
	if cancelErr := r.ctx.Err(); cancelErr != nil {
		return n, cancelErr
	}
	return n, err
}

type contextBody struct {
	contextReader
	closer io.Closer
}

func (b *contextBody) Close() error { return b.closer.Close() }
