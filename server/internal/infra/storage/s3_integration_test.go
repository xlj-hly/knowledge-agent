package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	appstorage "go-server/internal/app/storage"
)

// 真实服务回归仅通过 TEST_S3_ENABLED 显式启用，不加载 .env 或回退到 AWS 凭据链。
func TestS3IntegrationObjectLifecycle(t *testing.T) {
	if os.Getenv("TEST_S3_ENABLED") != "1" {
		t.Skip("set TEST_S3_ENABLED=1 and TEST_S3_* explicitly to run real S3 integration")
	}
	pathStyle, err := strconv.ParseBool(os.Getenv("TEST_S3_USE_PATH_STYLE"))
	if err != nil {
		t.Fatal("TEST_S3_USE_PATH_STYLE must be explicitly true or false")
	}
	config := Config{Provider: "s3", S3: S3Config{Endpoint: os.Getenv("TEST_S3_ENDPOINT"), Region: os.Getenv("TEST_S3_REGION"), Bucket: os.Getenv("TEST_S3_BUCKET"), AccessKeyID: os.Getenv("TEST_S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("TEST_S3_SECRET_ACCESS_KEY"), UsePathStyle: pathStyle}}
	store, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	key := "storage-integration/" + hex.EncodeToString(random[:]) + "/object.bin"
	// 在首次写入前登记清理，Put 返回故障时也清理可能已经写入的对象。
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := store.Delete(ctx, key); err != nil {
			t.Errorf("cleanup %s: %v", key, err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body := []byte{0, 255, 1, 2, 3, 128}
	if err := store.Put(ctx, key, bytes.NewBuffer(body), int64(len(body)), "application/octet-stream"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	metadata, err := store.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if metadata.Size != 6 || metadata.ContentType != "application/octet-stream" || metadata.ModifiedAt.IsZero() {
		t.Fatalf("incorrect stored metadata: %+v", metadata)
	}
	object, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	actual, readErr := io.ReadAll(object.Body)
	if err := errors.Join(readErr, object.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, body) || object.Metadata.Size != 6 || object.Metadata.ContentType != "application/octet-stream" {
		t.Fatalf("stored object mismatch: %v %+v", actual, object.Metadata)
	}
	if status, err := anonymousS3Get(ctx, config.S3, key); err != nil || status != http.StatusForbidden {
		t.Fatalf("private object anonymous access: status=%d error=%v", status, err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, appstorage.ErrNotFound) {
		t.Fatalf("Get after Delete: %v", err)
	}
	if _, err := store.Stat(ctx, key); !errors.Is(err, appstorage.ErrNotFound) {
		t.Fatalf("Stat after Delete: %v", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("repeated Delete: %v", err)
	}
}

func anonymousS3Get(ctx context.Context, config S3Config, key string) (int, error) {
	// 只复用官方 endpoint 解析；HTTP 请求绕过业务客户端的签名中间件，不携带凭据。
	parameters := s3.EndpointParameters{Bucket: aws.String(config.Bucket), Region: aws.String(config.Region), ForcePathStyle: aws.Bool(config.UsePathStyle)}
	if config.Endpoint != "" {
		parameters.Endpoint = aws.String(config.Endpoint)
	}
	endpoint, err := s3.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, parameters)
	if err != nil {
		return 0, errors.New("resolve anonymous S3 endpoint failed")
	}
	objectURL, err := url.JoinPath(endpoint.URI.String(), key)
	if err != nil {
		return 0, errors.New("construct anonymous S3 object URL failed")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return 0, errors.New("construct anonymous S3 request failed")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return 0, errors.New("anonymous S3 HTTP request failed")
	}
	return response.StatusCode, response.Body.Close()
}
