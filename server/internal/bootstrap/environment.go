package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type environmentError struct {
	path  string
	cause error
}

func (e *environmentError) Error() string {
	var fileError *os.PathError
	if errors.As(e.cause, &fileError) {
		return fmt.Sprintf("无法读取本地配置 %s，请检查文件权限", e.path)
	}
	return fmt.Sprintf("本地配置 %s 格式无效，请检查配置格式", e.path)
}

func (e *environmentError) Unwrap() error { return e.cause }

// LoadEnvironment 加载首个存在的本地配置，已设置的进程环境变量始终优先。
// 无文件时允许纯环境部署；解析失败不得把可能含密码的原始行写入启动日志。
func LoadEnvironment() error {
	workdir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("无法定位当前运行目录")
	}
	paths := []string{filepath.Join(workdir, ".env"), filepath.Join(workdir, "server", ".env")}
	if executable, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(executable), ".env"))
	}
	for _, path := range paths {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return &environmentError{path: path, cause: err}
		}
		if err := godotenv.Load(path); err != nil {
			return &environmentError{path: path, cause: err}
		}
		return nil
	}
	return nil
}
