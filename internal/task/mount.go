package task

import (
	"context"
	"errors"
	"os"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/pkg/i18n"
)

// MountConfig 分区挂载参数
type MountConfig struct {
	Disk       string // 整盘路径，如 /dev/sdb
	MountPoint string
	FSType     string
	Partition  string // 分区完成后填入
}

// PrepareMount 安装分区工具并列出可用磁盘
func PrepareMount(ctx context.Context) ([]sys.Disk, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New(i18n.T.Get("Please run with root privileges"))
	}
	info := sys.Detect()
	pkg := sys.NewPkg(info)
	if err := pkg.UseMirror(); err != nil {
		return nil, err
	}
	if err := pkg.Refresh(ctx); err != nil {
		return nil, err
	}
	pkgs := []string{"util-linux", "e2fsprogs", "xfsprogs"}
	if info.Family == sys.FamilyDebian {
		pkgs = append(pkgs, "fdisk")
	}
	if err := pkg.Install(ctx, pkgs...); err != nil {
		return nil, err
	}
	return sys.ListDisks(ctx)
}

// MountSteps 分区、格式化并挂载
func MountSteps(cfg *MountConfig) []Step {
	return []Step{
		{Name: i18n.T.Get("Preparing mount point"), Run: func(context.Context) error {
			if err := os.MkdirAll(cfg.MountPoint, 0o755); err != nil {
				return err
			}
			entries, err := os.ReadDir(cfg.MountPoint)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return errors.New(i18n.T.Get("Mount point is not empty"))
			}
			return nil
		}},
		{Name: i18n.T.Get("Partitioning disk"), Run: func(ctx context.Context) (err error) {
			cfg.Partition, err = sys.Partition(ctx, cfg.Disk)
			return err
		}},
		{Name: i18n.T.Get("Formatting partition"), Run: func(ctx context.Context) error {
			return sys.Format(ctx, cfg.Partition, cfg.FSType)
		}},
		{Name: i18n.T.Get("Mounting partition"), Run: func(ctx context.Context) error {
			return sys.Mount(ctx, cfg.Partition, cfg.MountPoint, cfg.FSType)
		}},
	}
}
