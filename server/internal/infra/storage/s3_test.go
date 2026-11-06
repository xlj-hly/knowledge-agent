package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/smithy-go"

	appstorage "go-server/internal/app/storage"
)

func testS3Config(endpoint string) Config {
	return Config{Provider: "s3", S3: S3Config{Endpoint: endpoint, Region: "us-east-1", Bucket: "test-bucket", AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key", UsePathStyle: true}}
}

func verifyS3Signature(t *testing.T, request *http.Request) bool {
	t.Helper()
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 Credential=test-access-key/") {
		t.Errorf("request did not use configured signing credentials")
		return false
	}
	signingTime, err := time.Parse("20060102T150405Z", request.Header.Get("X-Amz-Date"))
	if err != nil {
		t.Error(err)
		return false
	}
	clone := request.Clone(context.Background())
	clone.RequestURI = ""
	clone.URL.Scheme = "http"
	clone.URL.Host = request.Host
	clone.Header.Del("Authorization")
	err = v4.NewSigner().SignHTTP(context.Background(), aws.Credentials{AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key"}, clone, request.Header.Get("X-Amz-Content-Sha256"), "s3", "us-east-1", signingTime, func(options *v4.SignerOptions) { options.DisableURIPathEscaping = true })
	if err != nil {
		t.Error(err)
		return false
	}
	if clone.Header.Get("Authorization") != authorization {
		t.Error("request signature did not match configured region and secret")
		return false
	}
	return true
}

func TestS3HTTPStoresStreamsWithSignedRequests(t *testing.T) {
	var mutex sync.Mutex
	var body []byte
	var contentType string
	var exists bool
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !verifyS3Signature(t, request) {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		if request.URL.Path != "/test-bucket/avatars/1/picture.png" {
			t.Errorf("wrong configured bucket/key path: %q", request.URL.Path)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		switch request.Method {
		case http.MethodPut:
			var err error
			body, err = io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
				writer.WriteHeader(500)
				return
			}
			if request.ContentLength != 4 {
				t.Errorf("size not sent: %d", request.ContentLength)
			}
			contentType, exists = request.Header.Get("Content-Type"), true
			writer.WriteHeader(http.StatusOK)
		case http.MethodGet, http.MethodHead:
			if !exists {
				writer.Header().Set("Content-Type", "application/xml")
				writer.WriteHeader(http.StatusNotFound)
				if request.Method == http.MethodGet {
					_, _ = io.WriteString(writer, "<Error><Code>NoSuchKey</Code></Error>")
				}
				return
			}
			writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
			writer.Header().Set("Content-Type", contentType)
			writer.Header().Set("Last-Modified", modified.Format(http.TimeFormat))
			if request.Method == http.MethodGet {
				_, _ = writer.Write(body)
			}
		case http.MethodDelete:
			exists = false
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %s", request.Method)
			writer.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	store, err := New(testS3Config(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "avatars/1/picture.png"
	// bytes.Buffer 不是 io.Seeker，验证端口不会隐式要求可回放文件。
	if err := store.Put(ctx, key, bytes.NewBuffer([]byte{0, 1, 2, 255}), 4, "image/png"); err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Size != 4 || metadata.ContentType != "image/png" || !metadata.ModifiedAt.Equal(modified) {
		t.Fatalf("incorrect metadata: %+v", metadata)
	}
	object, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := io.ReadAll(object.Body)
	if err := errors.Join(readErr, object.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, []byte{0, 1, 2, 255}) || object.Metadata != metadata {
		t.Fatalf("incorrect object: %v %+v", actual, object.Metadata)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, appstorage.ErrNotFound) {
		t.Fatalf("missing Get: %v", err)
	}
	if _, err := store.Stat(ctx, key); !errors.Is(err, appstorage.ErrNotFound) {
		t.Fatalf("missing Stat: %v", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
}

func TestS3HTTPKeepsServiceFailuresAndRedactsDetails(t *testing.T) {
	for _, fixture := range []struct {
		code   string
		status int
	}{{"AccessDenied", 403}, {"NoSuchBucket", 404}} {
		t.Run(fixture.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/xml")
				writer.WriteHeader(fixture.status)
				_, _ = io.WriteString(writer, "<Error><Code>"+fixture.code+"</Code><Message>private-endpoint test-secret-key</Message></Error>")
			}))
			defer server.Close()
			store, err := New(testS3Config(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.Get(context.Background(), "object")
			var apiError smithy.APIError
			if err == nil || errors.Is(err, appstorage.ErrNotFound) || !errors.As(err, &apiError) || apiError.ErrorCode() != fixture.code {
				t.Fatalf("lost service failure: %v", err)
			}
			var diagnostic interface{ FailureCode() string }
			if !errors.As(err, &diagnostic) || diagnostic.FailureCode() != fixture.code {
				t.Fatalf("SDK service error lost diagnostic category: %v", err)
			}
			if strings.Contains(err.Error(), "test-secret-key") || strings.Contains(err.Error(), "private-endpoint") || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("error exposed private details: %v", err)
			}
		})
	}
}

func TestS3HTTPDoesNotTreatPermissionFailuresAsMissingObjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		writer.WriteHeader(http.StatusForbidden)
		if request.Method != http.MethodHead {
			_, _ = io.WriteString(writer, "<Error><Code>AccessDenied</Code></Error>")
		}
	}))
	defer server.Close()
	store, err := New(testS3Config(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	for name, operation := range map[string]func(context.Context) error{
		"put": func(ctx context.Context) error {
			return store.Put(ctx, "object", strings.NewReader("data"), 4, "text/plain")
		},
		"get":    func(ctx context.Context) error { _, err := store.Get(ctx, "object"); return err },
		"stat":   func(ctx context.Context) error { _, err := store.Stat(ctx, "object"); return err },
		"delete": func(ctx context.Context) error { return store.Delete(ctx, "object") },
	} {
		t.Run(name, func(t *testing.T) {
			err := operation(context.Background())
			var apiError smithy.APIError
			if err == nil || errors.Is(err, appstorage.ErrNotFound) || !errors.As(err, &apiError) {
				t.Fatalf("lost permission failure: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := operation(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
		})
	}
}

func TestS3HTTPCancellationPreservesContextAndDoesNotRetry(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { close(started); <-request.Context().Done() }))
	defer server.Close()
	store, err := New(testS3Config(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := store.Stat(ctx, "object"); result <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || errors.Is(err, appstorage.ErrNotFound) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled request did not return")
	}
}

func TestS3GetBodyHonorsCancellationAfterOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "4")
		_, _ = io.WriteString(writer, "data")
	}))
	defer server.Close()
	store, err := New(testS3Config(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	object, err := store.Get(ctx, "object")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_, readErr := io.ReadAll(object.Body)
	if err := object.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(readErr, context.Canceled) {
		t.Fatalf("body ignored cancellation: %v", readErr)
	}
}

func TestAnonymousS3ProbeSendsUnauthenticatedHTTPRequest(t *testing.T) {
	var requested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requested.Store(true)
		if request.Header.Get("Authorization") != "" || request.URL.Query().Get("X-Amz-Credential") != "" {
			t.Error("anonymous probe sent credentials")
		}
		if request.URL.Path != "/test-bucket/private/object.bin" {
			t.Errorf("incorrect anonymous object path: %q", request.URL.Path)
		}
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	status, err := anonymousS3Get(context.Background(), testS3Config(server.URL).S3, "private/object.bin")
	if err != nil {
		t.Fatalf("anonymous probe failed before reaching HTTP service: %v", err)
	}
	if !requested.Load() || status != http.StatusForbidden {
		t.Fatalf("anonymous denial not observed: requested=%v status=%d", requested.Load(), status)
	}
}

func TestS3RejectsLocalKeysAndInvalidConfigurationBeforeNetwork(t *testing.T) {
	store, err := New(testS3Config("http://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../escape", "/absolute", `C:\private`, `a\b`} {
		if err := store.Put(context.Background(), key, strings.NewReader("x"), 1, "text/plain"); !errors.Is(err, appstorage.ErrInvalidKey) {
			t.Fatalf("invalid Put: %v", err)
		}
		if _, err := store.Get(context.Background(), key); !errors.Is(err, appstorage.ErrInvalidKey) {
			t.Fatalf("invalid Get: %v", err)
		}
		if _, err := store.Stat(context.Background(), key); !errors.Is(err, appstorage.ErrInvalidKey) {
			t.Fatalf("invalid Stat: %v", err)
		}
		if err := store.Delete(context.Background(), key); !errors.Is(err, appstorage.ErrInvalidKey) {
			t.Fatalf("invalid Delete: %v", err)
		}
	}
	for _, endpoint := range []string{"ftp://host", "http://user:secret@host", "http://host?secret=value", "http://host#fragment", "missing-scheme"} {
		if store, err := New(testS3Config(endpoint)); err == nil || store != nil {
			t.Fatalf("invalid endpoint accepted")
		}
	}
	config := testS3Config("")
	if _, err := New(config); err != nil {
		t.Fatalf("standard AWS endpoint rejected: %v", err)
	}
}
