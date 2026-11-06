package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appstorage "go-server/internal/app/storage"
)

func TestFilesystemLifecyclePersistsContentAndMetadata(t *testing.T) {
	config := Config{Provider: "filesystem", FilesystemRoot: filepath.Join(t.TempDir(), "objects")}
	store, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "avatars/tenant-1/picture.png"
	body := []byte{0, 1, 2, 255}
	if err := store.Put(ctx, key, bytes.NewBuffer(body), int64(len(body)), "image/png"); err != nil {
		t.Fatal(err)
	}
	// 重新装配后仍能读取，防止依靠进程内元数据缓存通过测试。
	store, err = New(config)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Size != 4 || metadata.ContentType != "image/png" || metadata.ModifiedAt.IsZero() {
		t.Fatalf("incorrect metadata: %+v", metadata)
	}
	object, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := io.ReadAll(object.Body)
	closeErr := object.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, body) || object.Metadata != metadata {
		t.Fatalf("object mismatch: %v %+v", actual, object.Metadata)
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
		t.Fatalf("idempotent Delete: %v", err)
	}
}

func TestFilesystemRejectsInvalidKeysWithoutCreatingFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "objects")
	store, err := New(Config{Provider: "filesystem", FilesystemRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", ".", "..", "../outside", "a/../b", "/absolute", "a//b", "a/./b", "a/", `C:\private`, `a\b`, "file://host/path", "a\x00b", "a\nb", "CON", "x/aux.txt", "a. ", strings.Repeat("a", 1025)} {
		t.Run(key, func(t *testing.T) {
			ctx := context.Background()
			if err := store.Put(ctx, key, strings.NewReader("x"), 1, "text/plain"); !errors.Is(err, appstorage.ErrInvalidKey) {
				t.Fatalf("Put: %v", err)
			}
			if _, err := store.Get(ctx, key); !errors.Is(err, appstorage.ErrInvalidKey) {
				t.Fatalf("Get: %v", err)
			}
			if _, err := store.Stat(ctx, key); !errors.Is(err, appstorage.ErrInvalidKey) {
				t.Fatalf("Stat: %v", err)
			}
			if err := store.Delete(ctx, key); !errors.Is(err, appstorage.ErrInvalidKey) {
				t.Fatalf("Delete: %v", err)
			}
		})
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid key created root: %v", err)
	}
}

func TestFilesystemFailedReplacementKeepsPreviousObject(t *testing.T) {
	store, err := New(Config{Provider: "filesystem", FilesystemRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Put(ctx, "file", strings.NewReader("original"), 8, "text/plain"); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int64{1, 9, -1} {
		if err := store.Put(ctx, "file", strings.NewReader("bad"), size, "text/plain"); err == nil {
			t.Fatalf("accepted wrong length %d", size)
		}
	}
	object, err := store.Get(ctx, "file")
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := io.ReadAll(object.Body)
	if err := errors.Join(readErr, object.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if string(actual) != "original" {
		t.Fatalf("failed replacement damaged object: %q", actual)
	}
}

func TestFilesystemCancellationAndInfrastructureFailuresStayDistinct(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-root")
	if err := os.WriteFile(root, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := New(Config{Provider: "filesystem", FilesystemRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(ctx, "file", strings.NewReader("x"), 1, "text/plain"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put cancellation: %v", err)
	}
	if _, err := store.Get(ctx, "file"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get cancellation: %v", err)
	}
	if _, err := store.Stat(ctx, "file"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stat cancellation: %v", err)
	}
	if err := store.Delete(ctx, "file"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete cancellation: %v", err)
	}
	_, err = store.Stat(context.Background(), "file")
	var pathErr *os.PathError
	if err == nil || errors.Is(err, appstorage.ErrNotFound) || !errors.As(err, &pathErr) {
		t.Fatalf("infrastructure error lost: %v", err)
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("error leaks storage location: %v", err)
	}
}

func TestFilesystemInterruptedUploadLeavesNoPartialObject(t *testing.T) {
	root := t.TempDir()
	store, err := New(Config{Provider: "filesystem", FilesystemRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelingReader{cancel: cancel}
	if err := store.Put(ctx, "partial", reader, 1, "text/plain"); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted Put: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial upload or temporary file remains: %v", entries)
	}
}

type cancelingReader struct{ cancel context.CancelFunc }

func (r *cancelingReader) Read(body []byte) (int, error) { body[0] = 'x'; r.cancel(); return 1, nil }

func TestFilesystemGetBodyHonorsCancellationAfterOpen(t *testing.T) {
	store, err := New(Config{Provider: "filesystem", FilesystemRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "file", strings.NewReader("data"), 4, ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	object, err := store.Get(ctx, "file")
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
	if object.ContentType != "application/octet-stream" {
		t.Fatalf("wrong default content type %q", object.ContentType)
	}
}

func TestFilesystemRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "private"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink setup unavailable: %v", err)
	}
	store, err := New(Config{Provider: "filesystem", FilesystemRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "escape/file", strings.NewReader("x"), 1, "text/plain"); err == nil {
		t.Fatal("Put followed escaping symlink")
	}
	if _, err := store.Get(context.Background(), "escape/private"); err == nil {
		t.Fatal("Get followed escaping symlink")
	}
	if _, err := store.Stat(context.Background(), "escape/private"); err == nil {
		t.Fatal("Stat followed escaping symlink")
	}
	if err := store.Delete(context.Background(), "escape/private"); err == nil {
		t.Fatal("Delete followed escaping symlink")
	}
	actual, err := os.ReadFile(filepath.Join(outside, "private"))
	if err != nil || string(actual) != "secret" {
		t.Fatalf("outside file changed: %q %v", actual, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file created: %v", err)
	}
}

func TestFactoryRejectsUnconfiguredProviders(t *testing.T) {
	for _, config := range []Config{{}, {Provider: "unknown"}, {Provider: "filesystem"}, {Provider: "s3"}, {Provider: "s3", S3: S3Config{Region: "region", Bucket: "bucket", AccessKeyID: "key"}}} {
		if store, err := New(config); err == nil || store != nil {
			t.Fatalf("invalid config accepted: provider=%q", config.Provider)
		}
		if err := ValidateConfig(config); err == nil {
			t.Fatalf("invalid config passed validation: provider=%q", config.Provider)
		}
	}
}
