package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/acepanel/helper/pkg/i18n"
	"github.com/acepanel/helper/pkg/types"
)

// PackageManager 包管理器接口
type PackageManager interface {
	// UpdateCache 更新软件源缓存
	UpdateCache(ctx context.Context) error
	// Install 安装软件包
	Install(ctx context.Context, packages ...string) error
	// Remove 移除软件包
	Remove(ctx context.Context, packages ...string) error
	// IsInstalled 检查是否已安装
	IsInstalled(ctx context.Context, pkg string) bool
	// SetMirror 设置镜像源
	SetMirror(ctx context.Context, inChina bool) error
	// EnableEPEL 启用EPEL源 (仅RHEL系)
	EnableEPEL(ctx context.Context, inChina bool) error
}

// NewPackageManager 根据OS类型创建包管理器
func NewPackageManager(osType types.OSType, executor Executor) PackageManager {
	switch osType {
	case types.OSRHEL:
		return &dnfManager{executor: executor}
	case types.OSDebian, types.OSUbuntu:
		return &aptManager{executor: executor, osType: osType}
	default:
		return nil
	}
}

// dnfManager DNF包管理器 (RHEL系)
type dnfManager struct {
	executor Executor
}

func (m *dnfManager) UpdateCache(ctx context.Context) error {
	result, err := m.executor.Run(ctx, "dnf", "makecache", "-y")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T.Get("dnf makecache failed"), result.Stderr)
	}
	return nil
}

func (m *dnfManager) Install(ctx context.Context, packages ...string) error {
	args := append([]string{"install", "-y"}, packages...)
	result, err := m.executor.Run(ctx, "dnf", args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T.Get("dnf install failed"), result.Stderr)
	}
	return nil
}

func (m *dnfManager) Remove(ctx context.Context, packages ...string) error {
	args := append([]string{"remove", "-y"}, packages...)
	result, err := m.executor.Run(ctx, "dnf", args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T.Get("dnf remove failed"), result.Stderr)
	}
	return nil
}

func (m *dnfManager) IsInstalled(ctx context.Context, pkg string) bool {
	result, _ := m.executor.Run(ctx, "rpm", "-q", pkg)
	return result != nil && result.ExitCode == 0
}

func (m *dnfManager) SetMirror(ctx context.Context, inChina bool) error {
	if !inChina {
		return nil
	}

	// Rocky Linux
	m.sedReplace("/etc/yum.repos.d/[Rr]ocky*.repo",
		"s|^mirrorlist=|#mirrorlist=|g",
		"s|^#baseurl=http://dl.rockylinux.org/$contentdir|baseurl=https://mirrors.tencent.com/rocky|g",
	)

	// AlmaLinux
	m.sedReplace("/etc/yum.repos.d/[Aa]lmalinux*.repo",
		"s|^mirrorlist=|#mirrorlist=|g",
		"s|^#baseurl=https://repo.almalinux.org|baseurl=https://mirrors.tencent.com|g",
	)

	// CentOS Stream
	m.sedReplace("/etc/yum.repos.d/[Cc]ent*.repo",
		"s|^mirrorlist=|#mirrorlist=|g",
		"s|^#baseurl=http://mirror.centos.org/$contentdir|baseurl=https://mirrors.tencent.com/centos-stream|g",
	)

	return nil
}

func (m *dnfManager) EnableEPEL(ctx context.Context, inChina bool) error {
	// 启用CRB
	_, _ = m.executor.Run(ctx, "dnf", "config-manager", "--set-enabled", "crb")
	_, _ = m.executor.Run(ctx, "/usr/bin/crb", "enable")

	// 安装EPEL
	result, _ := m.executor.Run(ctx, "dnf", "install", "-y", "epel-release")
	if result == nil || result.ExitCode != 0 {
		// 手动安装
		var url string
		if inChina {
			url = "https://mirrors.tencent.com/epel/epel-release-latest-9.noarch.rpm"
		} else {
			url = "https://dl.fedoraproject.org/pub/epel/epel-release-latest-9.noarch.rpm"
		}
		_, _ = m.executor.Run(ctx, "dnf", "install", "-y", url)
	}

	if inChina {
		// 删除无镜像的repo
		_ = os.Remove("/etc/yum.repos.d/epel-cisco-openh264.repo")
		// 设置EPEL镜像
		m.sedReplace("/etc/yum.repos.d/epel*.repo",
			"s|^#baseurl=https://download.example/pub|baseurl=https://mirrors.tencent.com|g",
			"s|^metalink|#metalink|g",
		)
	}

	_, _ = m.executor.Run(ctx, "dnf", "config-manager", "--set-enabled", "epel")
	return nil
}

func (m *dnfManager) sedReplace(filePattern string, expressions ...string) {
	for _, expr := range expressions {
		_, _ = m.executor.Run(context.Background(), "sed", "-i", expr, filePattern)
	}
}

// aptManager APT包管理器 (Debian/Ubuntu)
type aptManager struct {
	executor Executor
	osType   types.OSType
}

func (m *aptManager) UpdateCache(ctx context.Context) error {
	result, err := m.executor.Run(ctx, "apt-get", "update", "-y")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T.Get("apt-get update failed"), result.Stderr)
	}
	return nil
}

func (m *aptManager) Install(ctx context.Context, packages ...string) error {
	args := append([]string{"install", "-y"}, packages...)
	result, err := m.executor.Run(ctx, "apt-get", args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T.Get("apt-get install failed"), result.Stderr)
	}
	return nil
}

func (m *aptManager) Remove(ctx context.Context, packages ...string) error {
	args := append([]string{"purge", "--auto-remove", "-y"}, packages...)
	result, err := m.executor.Run(ctx, "apt-get", args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s: %s", i18n.T.Get("apt-get remove failed"), result.Stderr)
	}
	return nil
}

func (m *aptManager) IsInstalled(ctx context.Context, pkg string) bool {
	result, _ := m.executor.Run(ctx, "dpkg", "-s", pkg)
	return result != nil && result.ExitCode == 0 && strings.Contains(result.Stdout, "Status: install ok installed")
}

func (m *aptManager) SetMirror(ctx context.Context, inChina bool) error {
	if !inChina {
		return nil
	}

	if m.osType == types.OSDebian {
		// Debian
		m.sedReplace("/etc/apt/sources.list", "s/deb.debian.org/mirrors.tencent.com/g")
		m.sedReplace("/etc/apt/sources.list.d/debian.sources", "s/deb.debian.org/mirrors.tencent.com/g")
		m.sedReplace("/etc/apt/sources.list",
			"s|security.debian.org/\\? |security.debian.org/debian-security |g",
			"s|security.debian.org|mirrors.tencent.com|g",
		)
	} else {
		// Ubuntu
		m.sedReplace("/etc/apt/sources.list", "s@//.*archive.ubuntu.com@//mirrors.tencent.com@g")
		m.sedReplace("/etc/apt/sources.list.d/ubuntu.sources", "s@//.*archive.ubuntu.com@//mirrors.tencent.com@g")
		m.sedReplace("/etc/apt/sources.list", "s/security.ubuntu.com/mirrors.tencent.com/g")
		m.sedReplace("/etc/apt/sources.list.d/ubuntu.sources", "s/security.ubuntu.com/mirrors.tencent.com/g")
	}

	return nil
}

func (m *aptManager) EnableEPEL(ctx context.Context, inChina bool) error {
	// APT不需要EPEL
	return nil
}

func (m *aptManager) sedReplace(file string, expressions ...string) {
	for _, expr := range expressions {
		_, _ = m.executor.Run(context.Background(), "sed", "-i", expr, file)
	}
}
