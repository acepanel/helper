package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/acepanel/helper/internal/system"
	"github.com/acepanel/helper/pkg/i18n"
	"github.com/acepanel/helper/pkg/types"
)

// Mounter 磁盘挂载器接口
type Mounter interface {
	ListDisks(ctx context.Context) ([]types.DiskInfo, error)
	IsPartitioned(disk string) bool
	Mount(ctx context.Context, cfg *types.MountConfig, progress ProgressCallback) error
	SetVerboseCallback(cb system.VerboseCallback)
}

type mounter struct {
	detector system.Detector
	executor system.Executor
}

// NewMounter 创建挂载器
func NewMounter(detector system.Detector, executor system.Executor) Mounter {
	return &mounter{
		detector: detector,
		executor: executor,
	}
}

func (m *mounter) SetVerboseCallback(cb system.VerboseCallback) {
	m.executor.SetVerboseCallback(cb)
}

func (m *mounter) ListDisks(ctx context.Context) ([]types.DiskInfo, error) {
	return m.detector.ListDisks(ctx)
}

func (m *mounter) IsPartitioned(disk string) bool {
	_, err := os.Stat("/dev/" + m.firstPartitionName(disk))
	return err == nil
}

func (m *mounter) Mount(ctx context.Context, cfg *types.MountConfig, progress ProgressCallback) error {
	partitionPath := "/dev/" + m.firstPartitionName(cfg.Disk)

	// 检查root权限
	if err := m.detector.CheckRoot(); err != nil {
		return err
	}

	// 检查磁盘是否存在
	if !m.detector.CheckDiskExists(cfg.Disk) {
		return errors.New(i18n.T.Get("Disk not found"))
	}

	// 检查是否为系统盘
	if m.detector.IsSystemDisk(cfg.Disk) {
		return errors.New(i18n.T.Get("Cannot operate on system disk"))
	}

	// 安装分区工具
	progress(i18n.T.Get("Installing partition tools"), i18n.T.Get("Installing partition tools..."))
	info, _ := m.detector.Detect(ctx)
	pkgMgr := system.NewPackageManager(info.OS, m.executor)
	if pkgMgr != nil {
		if info.OS == types.OSRHEL {
			if err := pkgMgr.Install(ctx, "xfsprogs", "e2fsprogs", "util-linux"); err != nil {
				return err
			}
		} else {
			if err := pkgMgr.Install(ctx, "xfsprogs", "e2fsprogs", "fdisk", "util-linux"); err != nil {
				return err
			}
		}
	}

	// 创建挂载点
	progress(i18n.T.Get("Creating mount point"), i18n.T.Get("Creating %s...", cfg.MountPoint))
	if err := os.MkdirAll(cfg.MountPoint, 0755); err != nil {
		return err
	}

	// 检查挂载点是否为空
	entries, err := os.ReadDir(cfg.MountPoint)
	if err == nil && len(entries) > 0 {
		return errors.New(i18n.T.Get("Mount point is not empty"))
	}

	// 卸载已有分区
	_, _ = m.executor.Run(ctx, "umount", partitionPath)

	// 删除所有分区
	progress(i18n.T.Get("Deleting existing partitions"), i18n.T.Get("Deleting existing partitions..."))
	fdiskInput := ""
	// 获取现有分区数
	result, _ := m.executor.Run(ctx, "lsblk", "-no", "NAME", "/dev/"+cfg.Disk)
	if result != nil {
		lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
		partCount := len(lines) - 1 // 减去磁盘本身
		for i := 0; i < partCount; i++ {
			fdiskInput += "d\n"
		}
	}
	fdiskInput += "w\n"
	_, _ = m.executor.RunWithInput(ctx, fdiskInput, "fdisk", "/dev/"+cfg.Disk)

	// 创建新分区
	progress(i18n.T.Get("Creating partition"), i18n.T.Get("Creating partition on /dev/%s...", cfg.Disk))
	partitionInput := "g\nn\n1\n\n\nw\n"
	_, err = m.executor.RunWithInput(ctx, partitionInput, "fdisk", "/dev/"+cfg.Disk)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to create partition"), err)
	}

	// 格式化
	progress(i18n.T.Get("Formatting partition"), i18n.T.Get("Formatting %s as %s...", partitionPath, cfg.FSType))
	switch cfg.FSType {
	case types.FSTypeExt4:
		result, err = m.executor.Run(ctx, "mkfs.ext4", "-F", partitionPath)
	case types.FSTypeXFS:
		result, err = m.executor.Run(ctx, "mkfs.xfs", "-f", partitionPath)
	default:
		return errors.New(i18n.T.Get("Unsupported filesystem type: %s", cfg.FSType))
	}
	if err != nil || (result != nil && result.ExitCode != 0) {
		return errors.New(i18n.T.Get("Format failed"))
	}

	// 重载systemd
	_, _ = m.executor.Run(ctx, "systemctl", "daemon-reload")

	// 挂载
	progress(i18n.T.Get("Mounting partition"), i18n.T.Get("Mounting %s to %s...", partitionPath, cfg.MountPoint))
	result, err = m.executor.Run(ctx, "mount", partitionPath, cfg.MountPoint)
	if err != nil || (result != nil && result.ExitCode != 0) {
		return errors.New(i18n.T.Get("Mount failed"))
	}

	// 获取UUID
	progress(i18n.T.Get("Updating fstab"), i18n.T.Get("Updating /etc/fstab for auto-mount..."))
	result, err = m.executor.Run(ctx, "blkid", "-s", "UUID", "-o", "value", partitionPath)
	if err != nil || result == nil || result.ExitCode != 0 {
		return errors.New(i18n.T.Get("Failed to get UUID"))
	}
	uuid := strings.TrimSpace(result.Stdout)

	// 更新fstab
	// 先删除旧条目
	_, _ = m.executor.Run(ctx, "sed", "-i", fmt.Sprintf("\\|%s|d", partitionPath), "/etc/fstab")
	_, _ = m.executor.Run(ctx, "sed", "-i", fmt.Sprintf("\\|%s|d", cfg.MountPoint), "/etc/fstab")

	// 添加新条目
	fstabEntry := fmt.Sprintf("UUID=%s    %s    %s    defaults    0    0\n", uuid, cfg.MountPoint, cfg.FSType)
	f, err := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func(f *os.File) { _ = f.Close() }(f)
	_, err = f.WriteString(fstabEntry)
	if err != nil {
		return err
	}

	// 重载并挂载
	_, _ = m.executor.Run(ctx, "systemctl", "daemon-reload")
	result, err = m.executor.Run(ctx, "mount", "-a")
	if err != nil || (result != nil && result.ExitCode != 0) {
		return errors.New(i18n.T.Get("fstab configuration error"))
	}

	progress(i18n.T.Get("Mount complete"), i18n.T.Get("Disk partition and mount successful"))
	return nil
}
func (m *mounter) firstPartitionName(disk string) string {
	if len(disk) == 0 {
		return ""
	}

	last := disk[len(disk)-1]
	if last >= '0' && last <= '9' {
		return disk + "p1"
	}
	return disk + "1"
}
