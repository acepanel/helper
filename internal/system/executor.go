package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/acepanel/helper/pkg/config"
)

// CommandResult 命令执行结果
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// VerboseCallback verbose 模式回调
type VerboseCallback func(cmd string)

// Executor 命令执行器接口
type Executor interface {
	// Run 执行命令并等待完成
	Run(ctx context.Context, name string, args ...string) (*CommandResult, error)
	// RunWithInput 执行命令并提供输入
	RunWithInput(ctx context.Context, input string, name string, args ...string) (*CommandResult, error)
	// RunStream 执行命令并流式输出
	RunStream(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
	// SetVerboseCallback 设置 verbose 回调
	SetVerboseCallback(cb VerboseCallback)
}

type executor struct {
	verboseCallback VerboseCallback
}

// NewExecutor 创建执行器
func NewExecutor() Executor {
	return &executor{}
}

func (e *executor) SetVerboseCallback(cb VerboseCallback) {
	e.verboseCallback = cb
}

func (e *executor) logVerbose(name string, args ...string) {
	if config.Global.Verbose && e.verboseCallback != nil {
		cmdStr := name
		if len(args) > 0 {
			cmdStr += " " + strings.Join(args, " ")
		}
		e.verboseCallback(cmdStr)
	}
}

func (e *executor) Run(ctx context.Context, name string, args ...string) (*CommandResult, error) {
	e.logVerbose(name, args...)

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		// 包含 stderr 信息到错误中
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return result, fmt.Errorf("%w: %s", err, stderrStr)
		}
		return result, err
	}

	return result, nil
}

func (e *executor) RunWithInput(ctx context.Context, input string, name string, args ...string) (*CommandResult, error) {
	e.logVerbose(name, args...)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewBufferString(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		// 包含 stderr 信息到错误中
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return result, fmt.Errorf("%w: %s", err, stderrStr)
		}
		return result, err
	}

	return result, nil
}

func (e *executor) RunStream(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	e.logVerbose(name, args...)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
