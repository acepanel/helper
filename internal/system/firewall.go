package system

import (
	"context"
	"fmt"

	"github.com/acepanel/helper/pkg/i18n"
)

// Firewall 防火墙接口
type Firewall interface {
	// Install 安装防火墙
	Install(ctx context.Context) error
	// Enable 启用防火墙
	Enable(ctx context.Context) error
	// AddPort 添加端口
	AddPort(ctx context.Context, port int, protocol string) error
	// RemovePort 移除端口
	RemovePort(ctx context.Context, port int, protocol string) error
	// Reload 重载配置
	Reload(ctx context.Context) error
}

type firewall struct {
	executor Executor
	detector Detector
}

// NewFirewall 创建防火墙管理器
func NewFirewall(executor Executor, detector Detector) Firewall {
	return &firewall{
		executor: executor,
		detector: detector,
	}
}

func (f *firewall) Install(ctx context.Context) error {
	info, err := f.detector.Detect(ctx)
	if err != nil {
		return err
	}
	pkgMgr := NewPackageManager(info.OS, f.executor)
	if pkgMgr == nil {
		return fmt.Errorf("%s", i18n.T().Get("Unsupported operating system"))
	}
	return pkgMgr.Install(ctx, "firewalld")
}

func (f *firewall) Enable(ctx context.Context) error {
	result, err := f.executor.Run(ctx, "systemctl", "enable", "--now", "firewalld")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T().Get("Failed to enable firewalld"), result.Stderr)
	}

	// 设置默认zone
	_, err = f.executor.Run(ctx, "firewall-cmd", "--set-default-zone=public")
	return err
}

func (f *firewall) AddPort(ctx context.Context, port int, protocol string) error {
	portStr := fmt.Sprintf("%d/%s", port, protocol)
	result, err := f.executor.Run(ctx, "firewall-cmd", "--permanent", "--zone=public", "--add-port="+portStr)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T().Get("Failed to add port"), portStr, result.Stderr)
	}
	return nil
}

func (f *firewall) RemovePort(ctx context.Context, port int, protocol string) error {
	portStr := fmt.Sprintf("%d/%s", port, protocol)
	result, err := f.executor.Run(ctx, "firewall-cmd", "--permanent", "--zone=public", "--remove-port="+portStr)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T().Get("Failed to remove port"), portStr, result.Stderr)
	}
	return nil
}

func (f *firewall) Reload(ctx context.Context) error {
	result, err := f.executor.Run(ctx, "firewall-cmd", "--reload")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T().Get("Failed to reload firewall"), result.Stderr)
	}
	return nil
}
