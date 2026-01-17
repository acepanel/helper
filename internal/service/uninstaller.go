package service

import (
	"context"
	"fmt"
	"os"

	"github.com/acepanel/helper/internal/system"
	"github.com/acepanel/helper/pkg/i18n"
)

// ProgressCallback 进度回调
type ProgressCallback func(step, message string)

// Uninstaller 卸载器接口
type Uninstaller interface {
	Uninstall(ctx context.Context, setupPath string, progress ProgressCallback) error
}

type uninstaller struct {
	detector system.Detector
	executor system.Executor
	systemd  system.Systemd
}

// NewUninstaller 创建卸载器
func NewUninstaller(
	detector system.Detector,
	executor system.Executor,
	systemd system.Systemd,
) Uninstaller {
	return &uninstaller{
		detector: detector,
		executor: executor,
		systemd:  systemd,
	}
}

func (u *uninstaller) Uninstall(ctx context.Context, setupPath string, progress ProgressCallback) error {
	// 检查root权限
	if err := u.detector.CheckRoot(); err != nil {
		return err
	}

	// 检查是否已安装
	if !u.detector.CheckPanelInstalled(setupPath) {
		return fmt.Errorf(i18n.T().Get("Panel is not installed"))
	}

	// 停止服务
	progress(i18n.T().Get("Stopping panel service"), i18n.T().Get("Stopping acepanel service..."))
	_ = u.systemd.Stop(ctx, "acepanel")
	_ = u.systemd.Disable(ctx, "acepanel")

	// 删除服务文件
	progress(i18n.T().Get("Removing service file"), i18n.T().Get("Removing systemd service file..."))
	_ = u.systemd.RemoveServiceFile("acepanel")
	_ = u.systemd.DaemonReload(ctx)

	// 删除CLI工具
	progress(i18n.T().Get("Removing CLI tool"), i18n.T().Get("Removing /usr/local/sbin/acepanel..."))
	_ = os.Remove("/usr/local/sbin/acepanel")

	// 移除swap
	progress(i18n.T().Get("Removing swap file"), i18n.T().Get("Removing swap file..."))
	swapFile := setupPath + "/swap"
	if _, err := os.Stat(swapFile); err == nil {
		_, _ = u.executor.Run(ctx, "swapoff", swapFile)
		_ = os.Remove(swapFile)
		// 从fstab中删除swap条目
		_, _ = u.executor.Run(ctx, "sed", "-i", "/swap/d", "/etc/fstab")
	}

	// 验证fstab
	result, _ := u.executor.Run(ctx, "mount", "-a")
	if result != nil && result.ExitCode != 0 {
		return fmt.Errorf(i18n.T().Get("fstab configuration error, please check /etc/fstab"))
	}

	// 删除安装目录
	progress(i18n.T().Get("Removing installation directory"), i18n.T().Get("Removing %s...", setupPath))
	_ = os.RemoveAll(setupPath)

	progress(i18n.T().Get("Uninstallation complete"), i18n.T().Get("Panel uninstalled successfully"))
	return nil
}
