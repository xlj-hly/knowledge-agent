// Package configuration 定义跨配置模块的逻辑关联写入边界。
package configuration

import "context"

// Guard 将同一企业的配置引用校验和写入放入同一工作单元。
// 回调只能包含本地校验和写入，不在锁内调用模型、向量库或其他外部服务。
type Guard interface {
	Within(context.Context, string, func(context.Context) error) error
}
