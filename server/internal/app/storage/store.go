// Package storage 定义应用使用的对象存储契约，与具体供应商和配置来源无关。
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound 表示对象不存在，不包含权限或基础设施故障。
var ErrNotFound = errors.New("object not found")

// ErrInvalidKey 表示对象键不是可移植的相对对象标识。
var ErrInvalidKey = errors.New("invalid object key")

// Metadata 是供应商无关的对象元数据。
type Metadata struct {
	Size        int64
	ContentType string
	ModifiedAt  time.Time
}

// Object 的 Body 由调用方关闭；读取期间仍受 Get 的上下文约束。
type Object struct {
	Body io.ReadCloser
	Metadata
}

// Store 存取完整对象；Put 成功后新内容可读，失败可能需要调用方清理。
// 键使用 / 分隔且不得包含绝对路径、空段、.、.. 或本地路径语法。
// Put 的 size 必须是精确字节数；空 ContentType 使用 application/octet-stream。
// Delete 幂等，删除不存在对象也成功；Get 和 Stat 用 ErrNotFound 表示缺失。
type Store interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (Object, error)
	Stat(ctx context.Context, key string) (Metadata, error)
	Delete(ctx context.Context, key string) error
}
