package sys

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Verbose 为真时把命令输出也交给 OnCommand
var Verbose bool

// OnCommand 命令开始时回调命令行；Verbose 模式下结束后再回调一次输出，此时 cmd 为空
var OnCommand func(cmd, output string)

var (
	logFile *os.File
	logMu   sync.Mutex
)

// OpenLog 打开命令日志文件，之后执行的命令与输出都追加写入
func OpenLog(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	logFile = f
	return nil
}

// CloseLog 关闭命令日志文件
func CloseLog() {
	if logFile != nil {
		_ = logFile.Close()
	}
}

// Exec 执行命令并返回输出和退出码，子进程放进独立进程组，上下文取消时整组杀掉
func Exec(ctx context.Context, input, name string, args ...string) (stdout, stderr string, code int, err error) {
	line := "$ " + strings.Join(append([]string{name}, args...), " ")
	if OnCommand != nil {
		OnCommand(line, "")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 3 * time.Second
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut

	err = cmd.Run()
	stdout, stderr = out.String(), errOut.String()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	record(line, stdout, stderr, err)
	return stdout, stderr, code, err
}

// Run 执行命令，退出码非零视为失败，错误里带上 stderr 末尾几行
func Run(ctx context.Context, name string, args ...string) (string, error) {
	return RunInput(ctx, "", name, args...)
}

// RunInput 带标准输入执行命令
func RunInput(ctx context.Context, input, name string, args ...string) (string, error) {
	stdout, stderr, _, err := Exec(ctx, input, name, args...)
	if err == nil {
		return stdout, nil
	}
	if msg := tail(stderr, 5); msg != "" {
		return stdout, fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return stdout, fmt.Errorf("%s: %w", name, err)
}

func record(line, stdout, stderr string, err error) {
	var output strings.Builder
	if s := strings.TrimSpace(stdout); s != "" {
		output.WriteString(s + "\n")
	}
	if s := strings.TrimSpace(stderr); s != "" {
		output.WriteString("[stderr] " + s + "\n")
	}
	if err != nil {
		output.WriteString("[error] " + err.Error() + "\n")
	}

	if logFile != nil {
		logMu.Lock()
		_, _ = fmt.Fprintf(logFile, "[%s] %s\n%s", time.Now().Format(time.DateTime), line, output.String())
		logMu.Unlock()
	}
	if Verbose && OnCommand != nil && output.Len() > 0 {
		OnCommand("", strings.TrimRight(output.String(), "\n"))
	}
}

// tail 取文本末尾 n 行
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
