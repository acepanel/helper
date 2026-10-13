package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/acepanel/helper/pkg/i18n"
)

// Disk 可用于分区的整盘
type Disk struct {
	Name string
	Path string
	Size int64
}

type blockDevice struct {
	Name        string        `json:"name"`
	Path        string        `json:"path"`
	Size        int64         `json:"size"`
	Type        string        `json:"type"`
	FSType      string        `json:"fstype"`
	MountPoints []*string     `json:"mountpoints"`
	Children    []blockDevice `json:"children"`
}

// inUse 自身或任一后代被挂载、作 swap、属于 LVM/RAID/LUKS 都算在用
func (d blockDevice) inUse() bool {
	for _, mp := range d.MountPoints {
		if mp != nil && *mp != "" {
			return true
		}
	}
	switch d.FSType {
	case "LVM2_member", "crypto_LUKS", "linux_raid_member", "swap":
		return true
	}
	for _, c := range d.Children {
		if c.Type != "part" || c.inUse() {
			return true
		}
	}
	return false
}

// ListDisks 列出没有被使用的整盘
func ListDisks(ctx context.Context) ([]Disk, error) {
	out, err := Run(ctx, "lsblk", "-J", "-b", "-o", "NAME,PATH,SIZE,TYPE,FSTYPE,MOUNTPOINTS")
	if err != nil {
		return nil, err
	}
	var tree struct {
		BlockDevices []blockDevice `json:"blockdevices"`
	}
	if err = json.Unmarshal([]byte(out), &tree); err != nil {
		return nil, err
	}
	var disks []Disk
	for _, d := range tree.BlockDevices {
		if d.Type != "disk" || d.Size == 0 || strings.HasPrefix(d.Name, "zram") || d.inUse() {
			continue
		}
		disks = append(disks, Disk{Name: d.Name, Path: d.Path, Size: d.Size})
	}
	return disks, nil
}

// Partition 清空磁盘并建一个占满整盘的 GPT 分区，返回分区路径
func Partition(ctx context.Context, disk string) (string, error) {
	if _, err := RunInput(ctx, "label: gpt\n,,L\n", "sfdisk", "-q", "--wipe", "always", "--wipe-partitions", "always", disk); err != nil {
		return "", err
	}
	_, _ = Run(ctx, "udevadm", "settle")
	out, err := Run(ctx, "lsblk", "-nro", "PATH,TYPE", disk)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[1] == "part" {
			return fields[0], nil
		}
	}
	return "", errors.New(i18n.T.Get("Partition not found after partitioning"))
}

// Format 格式化分区
func Format(ctx context.Context, part, fstype string) error {
	var err error
	switch fstype {
	case "ext4":
		_, err = Run(ctx, "mkfs.ext4", "-F", part)
	case "xfs":
		_, err = Run(ctx, "mkfs.xfs", "-f", part)
	default:
		return errors.New(i18n.T.Get("Unsupported filesystem type: %s", fstype))
	}
	return err
}

// Mount 挂载分区并按 UUID 写入 fstab，同一挂载点或同一分区的旧条目会被替换
func Mount(ctx context.Context, part, mountPoint, fstype string) error {
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		return err
	}
	if _, err := Run(ctx, "mount", part, mountPoint); err != nil {
		return err
	}
	out, err := Run(ctx, "blkid", "-s", "UUID", "-o", "value", part)
	if err != nil {
		return err
	}
	uuid := "UUID=" + strings.TrimSpace(out)
	entry := fmt.Sprintf("%s\t%s\t%s\tdefaults\t0\t0", uuid, mountPoint, fstype)
	if err = EditFstab(func(f []string) bool { return f[0] != uuid && f[1] != mountPoint }, entry); err != nil {
		return err
	}
	_ = Systemctl(ctx, "daemon-reload")
	_, err = Run(ctx, "mount", "-a")
	return err
}

// CreateSwap 创建并启用 swap 文件，同时写入 fstab
func CreateSwap(ctx context.Context, file string, sizeMB int) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	size := strconv.Itoa(sizeMB) + "M"
	// btrfs 上普通文件不能做 swap，要用专用命令创建
	fsType, _ := Run(ctx, "findmnt", "-no", "FSTYPE", "-T", filepath.Dir(file))
	if strings.TrimSpace(fsType) == "btrfs" {
		if _, err := Run(ctx, "btrfs", "filesystem", "mkswapfile", "--size", size, "--uuid", "clear", file); err != nil {
			return err
		}
	} else if _, err := Run(ctx, "fallocate", "-l", size, file); err != nil {
		if _, err = Run(ctx, "dd", "if=/dev/zero", "of="+file, "bs=1M", "count="+strconv.Itoa(sizeMB)); err != nil {
			return err
		}
	}
	if err := os.Chmod(file, 0o600); err != nil {
		return err
	}
	if _, err := Run(ctx, "mkswap", "-f", file); err != nil {
		return err
	}
	if _, err := Run(ctx, "swapon", file); err != nil {
		return err
	}
	return EditFstab(func(f []string) bool { return f[0] != file }, file+"\tnone\tswap\tdefaults\t0\t0")
}

// RemoveSwap 关闭并删除 swap 文件及其 fstab 条目
func RemoveSwap(ctx context.Context, file string) error {
	if _, err := os.Stat(file); err != nil {
		return nil
	}
	_, _ = Run(ctx, "swapoff", file)
	if err := os.Remove(file); err != nil {
		return err
	}
	return EditFstab(func(f []string) bool { return f[0] != file }, "")
}
