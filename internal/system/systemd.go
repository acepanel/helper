package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/acepanel/helper/pkg/i18n"
)

// Systemd systemd服务管理接口
type Systemd interface {
	// Start 启动服务
	Start(ctx context.Context, service string) error
	// Stop 停止服务
	Stop(ctx context.Context, service string) error
	// Enable 启用服务
	Enable(ctx context.Context, service string) error
	// Disable 禁用服务
	Disable(ctx context.Context, service string) error
	// Restart 重启服务
	Restart(ctx context.Context, service string) error
	// IsActive 检查服务是否运行
	IsActive(ctx context.Context, service string) bool
	// DaemonReload 重载systemd配置
	DaemonReload(ctx context.Context) error
	// WriteServiceFile 写入服务文件
	WriteServiceFile(name string, content string) error
	// RemoveServiceFile 删除服务文件
	RemoveServiceFile(name string) error
}

type systemd struct {
	executor Executor
}

// NewSystemd 创建systemd管理器
func NewSystemd(executor Executor) Systemd {
	return &systemd{executor: executor}
}

func (s *systemd) Start(ctx context.Context, service string) error {
	result, err := s.executor.Run(ctx, "systemctl", "start", service)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T.Get("Failed to start"), service, result.Stderr)
	}
	return nil
}

func (s *systemd) Stop(ctx context.Context, service string) error {
	result, err := s.executor.Run(ctx, "systemctl", "stop", service)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T.Get("Failed to stop"), service, result.Stderr)
	}
	return nil
}

func (s *systemd) Enable(ctx context.Context, service string) error {
	result, err := s.executor.Run(ctx, "systemctl", "enable", "--now", service)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T.Get("Failed to enable"), service, result.Stderr)
	}
	return nil
}

func (s *systemd) Disable(ctx context.Context, service string) error {
	result, err := s.executor.Run(ctx, "systemctl", "disable", service)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T.Get("Failed to disable"), service, result.Stderr)
	}
	return nil
}

func (s *systemd) Restart(ctx context.Context, service string) error {
	result, err := s.executor.Run(ctx, "systemctl", "restart", service)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T.Get("Failed to restart"), service, result.Stderr)
	}
	return nil
}

func (s *systemd) IsActive(ctx context.Context, service string) bool {
	result, _ := s.executor.Run(ctx, "systemctl", "is-active", service)
	return result != nil && strings.TrimSpace(result.Stdout) == "active"
}

func (s *systemd) DaemonReload(ctx context.Context) error {
	result, err := s.executor.Run(ctx, "systemctl", "daemon-reload")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T.Get("Failed to daemon-reload"), result.Stderr)
	}
	return nil
}

func (s *systemd) WriteServiceFile(name string, content string) error {
	path := fmt.Sprintf("/etc/systemd/system/%s.service", name)
	return os.WriteFile(path, []byte(content), 0644)
}

func (s *systemd) RemoveServiceFile(name string) error {
	path := fmt.Sprintf("/etc/systemd/system/%s.service", name)
	return os.Remove(path)
}
