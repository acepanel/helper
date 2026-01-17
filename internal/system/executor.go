package system

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
)

// CommandResult 命令执行结果
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Executor 命令执行器接口
type Executor interface {
	// Run 执行命令并等待完成
	Run(ctx context.Context, name string, args ...string) (*CommandResult, error)
	// RunWithInput 执行命令并提供输入
	RunWithInput(ctx context.Context, input string, name string, args ...string) (*CommandResult, error)
	// RunStream 执行命令并流式输出
	RunStream(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
}

type executor struct{}

// NewExecutor 创建执行器
func NewExecutor() Executor {
	return &executor{}
}

func (e *executor) Run(ctx context.Context, name string, args ...string) (*CommandResult, error) {
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
		return result, err
	}

	return result, nil
}

func (e *executor) RunWithInput(ctx context.Context, input string, name string, args ...string) (*CommandResult, error) {
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
		return result, err
	}

	return result, nil
}

func (e *executor) RunStream(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
