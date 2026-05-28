package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

const (
	maxLogSize    int64 = 10 * 1024 * 1024
	maxLogBackups       = 3
)

type rotatingWriter struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	file    *os.File
	size    int64
}

func newRotating(path string, maxSize int64, backups int) (*rotatingWriter, error) {
	w := &rotatingWriter{path: path, maxSize: maxSize, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) open() error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingWriter) rotate() error {
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
	os.Remove(fmt.Sprintf("%s.%d", w.path, w.backups))
	for i := w.backups - 1; i >= 1; i-- {
		os.Rename(
			fmt.Sprintf("%s.%d", w.path, i),
			fmt.Sprintf("%s.%d", w.path, i+1),
		)
	}
	if w.backups > 0 {
		os.Rename(w.path, w.path+".1")
	} else {
		os.Remove(w.path)
	}
	return w.open()
}

// Init 把所有日志输出汇聚到滚动文件，并接管 stdout/stderr：
//   - slog 仅输出 Error 级别，直接写入滚动文件
//   - 通过 dup3 把 fd 1、fd 2 重定向到管道，由 goroutine 转发进滚动文件
//     这会拦截 gin 访问日志、fmt.Print*、panic、stdlib log 等所有 stdio 输出
//   - 调用后 shell 外部重定向（如 `./webssh >> /tmp/xxx.log 2>&1`）不再生效
//
// 日志文件超过 10MB 自动滚动，最多保留 3 个备份（.1 .2 .3）。
func Init(logPath string) {
	rw, err := newRotating(logPath, maxLogSize, maxLogBackups)
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
		slog.Error("初始化日志文件失败", "err", err.Error(), "path", logPath)
		return
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(rw, &slog.HandlerOptions{Level: slog.LevelError})))

	r, w, err := os.Pipe()
	if err != nil {
		slog.Error("创建日志管道失败", "err", err.Error())
		return
	}
	pipeFd := int(w.Fd())
	if err := syscall.Dup3(pipeFd, 1, 0); err != nil {
		slog.Error("重定向 stdout 失败", "err", err.Error())
		return
	}
	if err := syscall.Dup3(pipeFd, 2, 0); err != nil {
		slog.Error("重定向 stderr 失败", "err", err.Error())
		return
	}
	w.Close()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				_, _ = rw.Write(buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}()
}
