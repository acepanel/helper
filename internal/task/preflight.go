package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/pkg/i18n"
)

// Preflight 安装前检测结果
type Preflight struct {
	Info     *sys.Info
	Outdated bool   // 不是最新小版本
	Latest   string // 可升级到的小版本，解析不出时为空
}

// PreflightSteps 安装前检测：系统要求、软件源、发行版小版本
func PreflightSteps(setupPath string, r *Preflight) []Step {
	return []Step{
		{Name: i18n.T.Get("Checking system requirements"), Run: func(context.Context) error {
			r.Info = sys.Detect()
			return checkSystem(r.Info, setupPath)
		}},
		{Name: i18n.T.Get("Updating package index"), Run: func(ctx context.Context) error {
			pkg := sys.NewPkg(r.Info)
			if err := pkg.UseMirror(); err != nil {
				return err
			}
			return pkg.Refresh(ctx)
		}},
		{Name: i18n.T.Get("Checking system release"), Run: func(ctx context.Context) (err error) {
			r.Outdated, r.Latest, err = sys.NewPkg(r.Info).ReleaseUpdate(ctx)
			return err
		}},
	}
}

// UpgradeSteps 升级系统软件包
func UpgradeSteps(info *sys.Info) []Step {
	return []Step{
		{Name: i18n.T.Get("Upgrading system packages"), Run: func(ctx context.Context) error {
			return sys.NewPkg(info).Upgrade(ctx)
		}},
	}
}

func checkSystem(info *sys.Info, setupPath string) error {
	if os.Geteuid() != 0 {
		return errors.New(i18n.T.Get("Please run with root privileges"))
	}
	if info.Family == "" {
		return errors.New(i18n.T.Get("Unsupported operating system"))
	}
	if major, _ := strconv.Atoi(strings.SplitN(info.Kernel, ".", 2)[0]); major < 5 {
		return errors.New(i18n.T.Get("Kernel version too old, requires 5.x or above"))
	}
	if err := checkCPU(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(setupPath, "panel", "ace")); err == nil {
		return errors.New(i18n.T.Get("Panel is already installed"))
	}
	return nil
}

// checkCPU 面板里的应用按 x86-64-v2 编译，缺 SSE4.2/POPCNT 的老 CPU 跑不起来
func checkCPU() error {
	if runtime.GOARCH != "amd64" {
		return nil
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return err
	}
	for _, flag := range []string{"ssse3", "sse4_2", "popcnt"} {
		if !strings.Contains(string(data), " "+flag) {
			return errors.New(i18n.T.Get("CPU must support at least x86-64-v2 instruction set"))
		}
	}
	return nil
}
