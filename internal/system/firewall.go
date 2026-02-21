package system

import (
	"context"
	"fmt"

	"github.com/acepanel/helper/pkg/types"
)

// Firewall 防火墙接口
type Firewall interface {
	// Install 安装防火墙
	Install(ctx context.Context)
	// Enable 启用防火墙
	Enable(ctx context.Context)
	// AddPort 添加端口
	AddPort(ctx context.Context, port int, protocol string)
	// RemovePort 移除端口
	RemovePort(ctx context.Context, port int, protocol string)
	// Reload 重载配置
	Reload(ctx context.Context)
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

func (f *firewall) Install(ctx context.Context) {
	info, err := f.detector.Detect(ctx)
	if err != nil {
		return
	}
	pkgMgr := NewPackageManager(info.OS, f.executor)
	if pkgMgr == nil {
		return
	}

	if f.isUFW(ctx) {
		_ = pkgMgr.Install(ctx, "ufw")
	} else {
		_ = pkgMgr.Install(ctx, "firewalld")
	}
}

func (f *firewall) Enable(ctx context.Context) {
	if f.isUFW(ctx) {
		_, _ = f.executor.Run(ctx, "ufw", "--force", "enable")
	} else {
		_, _ = f.executor.Run(ctx, "systemctl", "enable", "--now", "firewalld")
		_, _ = f.executor.Run(ctx, "firewall-cmd", "--set-default-zone=public")
	}
}

func (f *firewall) AddPort(ctx context.Context, port int, protocol string) {
	portStr := fmt.Sprintf("%d/%s", port, protocol)

	if f.isUFW(ctx) {
		_, _ = f.executor.Run(ctx, "ufw", "allow", portStr)
	} else {
		_, _ = f.executor.Run(ctx, "firewall-cmd", "--permanent", "--zone=public", "--add-port="+portStr)
	}
}

func (f *firewall) RemovePort(ctx context.Context, port int, protocol string) {
	portStr := fmt.Sprintf("%d/%s", port, protocol)

	if f.isUFW(ctx) {
		_, _ = f.executor.Run(ctx, "ufw", "delete", "allow", portStr)
	} else {
		_, _ = f.executor.Run(ctx, "firewall-cmd", "--permanent", "--zone=public", "--remove-port="+portStr)
	}
}

func (f *firewall) Reload(ctx context.Context) {
	if f.isUFW(ctx) {
		_, _ = f.executor.Run(ctx, "ufw", "reload")
	} else {
		_, _ = f.executor.Run(ctx, "firewall-cmd", "--reload")
	}
}

func (f *firewall) isUFW(ctx context.Context) bool {
	info, err := f.detector.Detect(ctx)
	if err != nil {
		return false
	}
	return info.OS == types.OSDebian || info.OS == types.OSUbuntu
}
