package storage

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"

	appstorage "go-server/internal/app/storage"
)

// 单文件封装使内容与 ContentType 在一次 Rename 中提交，避免旁路元数据失配。
var objectMagic = [4]byte{'O', 'B', 'J', 1}

type filesystemStore struct{ root string }

func (s *filesystemStore) openRoot(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(s.root, 0700); err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(s.root)
}

func closeRoot(root *os.Root, result *error, operation string) {
	if err := root.Close(); err != nil {
		*result = errors.Join(*result, failed("filesystem", operation, err))
	}
}

func (s *filesystemStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (result error) {
	if err := checkRequest(ctx, key); err != nil {
		return err
	}
	if body == nil || size < 0 {
		return errors.New("invalid object body or size")
	}
	contentType, err := contentTypeOrDefault(contentType)
	if err != nil {
		return err
	}
	root, err := s.openRoot(true)
	if err != nil {
		return failed("filesystem", "put", err)
	}
	defer closeRoot(root, &result, "put")
	if err := root.MkdirAll(path.Dir(key), 0700); err != nil {
		return failed("filesystem", "put", err)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return failed("filesystem", "put", err)
	}
	temporary := path.Join(path.Dir(key), ".upload-"+hex.EncodeToString(random[:]))
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return failed("filesystem", "put", err)
	}
	committed := false
	defer func() {
		if !committed {
			if err := root.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
				result = errors.Join(result, failed("filesystem", "cleanup", err))
			}
		}
	}()
	writeErr := writeObject(ctx, file, body, size, contentType)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return failed("filesystem", "put", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.Rename(temporary, key); err != nil {
		return failed("filesystem", "put", err)
	}
	committed = true
	return nil
}

func writeObject(ctx context.Context, file *os.File, body io.Reader, size int64, contentType string) error {
	header := make([]byte, 8+len(contentType))
	copy(header, objectMagic[:])
	binary.BigEndian.PutUint32(header[4:8], uint32(len(contentType)))
	copy(header[8:], contentType)
	if _, err := file.Write(header); err != nil {
		return err
	}
	reader := &contextReader{ctx: ctx, reader: body}
	n, err := io.Copy(file, io.LimitReader(reader, size))
	if err != nil {
		return err
	}
	if n != size {
		return errors.New("object body size mismatch")
	}
	var extra [1]byte
	next, err := reader.Read(extra[:])
	if next != 0 || err == nil {
		return errors.New("object body size mismatch")
	}
	if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (s *filesystemStore) openObject(key string) (*os.File, appstorage.Metadata, error) {
	root, err := s.openRoot(false)
	if err != nil {
		return nil, appstorage.Metadata{}, err
	}
	file, openErr := root.Open(key)
	rootErr := root.Close()
	if openErr != nil {
		return nil, appstorage.Metadata{}, errors.Join(openErr, rootErr)
	}
	metadata, metadataErr := readMetadata(file)
	if err := errors.Join(rootErr, metadataErr); err != nil {
		return nil, appstorage.Metadata{}, errors.Join(err, file.Close())
	}
	return file, metadata, nil
}

func readMetadata(file *os.File) (appstorage.Metadata, error) {
	info, err := file.Stat()
	if err != nil {
		return appstorage.Metadata{}, err
	}
	if !info.Mode().IsRegular() {
		return appstorage.Metadata{}, appstorage.ErrNotFound
	}
	var header [8]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return appstorage.Metadata{}, err
	}
	if string(header[:4]) != string(objectMagic[:]) {
		return appstorage.Metadata{}, errors.New("invalid object metadata")
	}
	length := binary.BigEndian.Uint32(header[4:])
	if length > 1024 || info.Size() < 8+int64(length) {
		return appstorage.Metadata{}, errors.New("invalid object metadata")
	}
	contentType := make([]byte, length)
	if _, err := io.ReadFull(file, contentType); err != nil {
		return appstorage.Metadata{}, err
	}
	return appstorage.Metadata{Size: info.Size() - 8 - int64(length), ContentType: string(contentType), ModifiedAt: info.ModTime()}, nil
}

func filesystemError(operation string, err error) error {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, appstorage.ErrNotFound) {
		return appstorage.ErrNotFound
	}
	return failed("filesystem", operation, err)
}

func (s *filesystemStore) Get(ctx context.Context, key string) (appstorage.Object, error) {
	if err := checkRequest(ctx, key); err != nil {
		return appstorage.Object{}, err
	}
	file, metadata, err := s.openObject(key)
	if err != nil {
		return appstorage.Object{}, filesystemError("get", err)
	}
	return appstorage.Object{Body: &contextBody{contextReader: contextReader{ctx: ctx, reader: file}, closer: file}, Metadata: metadata}, nil
}

func (s *filesystemStore) Stat(ctx context.Context, key string) (appstorage.Metadata, error) {
	if err := checkRequest(ctx, key); err != nil {
		return appstorage.Metadata{}, err
	}
	file, metadata, err := s.openObject(key)
	if err != nil {
		return appstorage.Metadata{}, filesystemError("stat", err)
	}
	if err := file.Close(); err != nil {
		return appstorage.Metadata{}, failed("filesystem", "stat", err)
	}
	return metadata, nil
}

func (s *filesystemStore) Delete(ctx context.Context, key string) (result error) {
	if err := checkRequest(ctx, key); err != nil {
		return err
	}
	root, err := s.openRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return failed("filesystem", "delete", err)
	}
	defer closeRoot(root, &result, "delete")
	if err := root.Remove(key); err != nil && !errors.Is(err, os.ErrNotExist) {
		return failed("filesystem", "delete", err)
	}
	return nil
}
