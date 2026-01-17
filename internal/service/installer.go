package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-resty/resty/v2"

	"github.com/acepanel/helper/internal/system"
	"github.com/acepanel/helper/pkg/i18n"
	"github.com/acepanel/helper/pkg/types"
)

// Installer 安装器接口
type Installer interface {
	Install(ctx context.Context, cfg *types.InstallConfig, progress chan<- types.Progress) error
}

type installer struct {
	detector system.Detector
	executor system.Executor
	firewall system.Firewall
	systemd  system.Systemd
	userMgr  system.UserManager
}

// NewInstaller 创建安装器
func NewInstaller(
	detector system.Detector,
	executor system.Executor,
	firewall system.Firewall,
	systemd system.Systemd,
	userMgr system.UserManager,
) Installer {
	return &installer{
		detector: detector,
		executor: executor,
		firewall: firewall,
		systemd:  systemd,
		userMgr:  userMgr,
	}
}

func (i *installer) Install(ctx context.Context, cfg *types.InstallConfig, progress chan<- types.Progress) error {
	steps := []struct {
		name   string
		weight float64
		fn     func(ctx context.Context, cfg *types.InstallConfig) error
	}{
		{i18n.T().Get("Checking system requirements"), 0.05, i.checkSystem},
		{i18n.T().Get("Creating www user"), 0.02, i.createUser},
		{i18n.T().Get("Optimizing system settings"), 0.08, i.optimizeSystem},
		{i18n.T().Get("Installing dependencies"), 0.20, i.installDeps},
		{i18n.T().Get("Creating swap file"), 0.05, i.createSwap},
		{i18n.T().Get("Downloading panel"), 0.30, i.downloadPanel},
		{i18n.T().Get("Configuring firewall"), 0.10, i.configureFirewall},
		{i18n.T().Get("Creating systemd service"), 0.10, i.createService},
		{i18n.T().Get("Initializing panel"), 0.08, i.initPanel},
		{i18n.T().Get("Detecting installed apps"), 0.02, i.detectApps},
	}

	var currentProgress float64
	for _, step := range steps {
		progress <- types.Progress{
			Step:    step.name,
			Percent: currentProgress,
			Message: step.name + "...",
		}

		if err := step.fn(ctx, cfg); err != nil {
			progress <- types.Progress{
				Step:    step.name,
				Percent: currentProgress,
				Message: fmt.Sprintf("%s: %v", i18n.T().Get("Error"), err),
				IsError: true,
				Error:   err,
			}
			return err
		}

		currentProgress += step.weight
		progress <- types.Progress{
			Step:    step.name,
			Percent: currentProgress,
			Message: step.name + " " + i18n.T().Get("completed"),
		}
	}

	progress <- types.Progress{
		Step:    i18n.T().Get("Installation complete"),
		Percent: 1.0,
		Message: i18n.T().Get("Panel installed successfully"),
	}

	return nil
}

func (i *installer) checkSystem(ctx context.Context, cfg *types.InstallConfig) error {
	// 检查root权限
	if err := i.detector.CheckRoot(); err != nil {
		return err
	}

	// 检测系统信息
	info, err := i.detector.Detect(ctx)
	if err != nil {
		return err
	}

	// 检查OS
	if info.OS == types.OSUnknown {
		return errors.New(i18n.T().Get("Unsupported operating system"))
	}

	// 检查架构
	if info.Arch == types.ArchUnknown {
		return errors.New(i18n.T().Get("Unsupported CPU architecture"))
	}

	// 检查CPU特性
	if err := i.detector.CheckCPUFeatures(ctx); err != nil {
		return err
	}

	// 检查内核版本
	if info.KernelVersion != "" {
		parts := strings.Split(info.KernelVersion, ".")
		if len(parts) > 0 {
			major := 0
			_, _ = fmt.Sscanf(parts[0], "%d", &major)
			if major < 4 {
				return errors.New(i18n.T().Get("Kernel version too old, requires 4.x or above"))
			}
		}
	}

	// 检查是否64位
	if !info.Is64Bit {
		return errors.New(i18n.T().Get("Requires 64-bit system"))
	}

	// 检查是否已安装
	if i.detector.CheckPanelInstalled(cfg.SetupPath) {
		return errors.New(i18n.T().Get("Panel is already installed"))
	}

	// 保存系统信息到配置
	cfg.InChina = info.InChina

	return nil
}

func (i *installer) createUser(ctx context.Context, cfg *types.InstallConfig) error {
	return i.userMgr.EnsureUserAndGroup(ctx, "www", "www")
}

func (i *installer) optimizeSystem(ctx context.Context, cfg *types.InstallConfig) error {
	// 设置时区
	_, _ = i.executor.Run(ctx, "timedatectl", "set-timezone", "Asia/Shanghai")

	// 禁用SELinux
	_, _ = i.executor.Run(ctx, "setenforce", "0")
	_, _ = i.executor.Run(ctx, "sed", "-i", "s/SELINUX=enforcing/SELINUX=disabled/g", "/etc/selinux/config")

	// 系统参数优化
	_, _ = i.executor.Run(ctx, "sysctl", "-w", "vm.overcommit_memory=1")
	_, _ = i.executor.Run(ctx, "sysctl", "-w", "net.core.somaxconn=1024")

	// 写入sysctl配置
	sysctlConf := `fs.file-max = 2147483584
net.core.somaxconn = 1024
net.ipv4.tcp_congestion_control = bbr
`
	_ = os.WriteFile("/etc/sysctl.d/99-panel.conf", []byte(sysctlConf), 0644)

	// 写入limits配置
	limitsConf := `* soft nofile 1048576
* hard nofile 1048576
* soft nproc 1048576
* hard nproc 1048576
`
	// 追加到limits.conf
	f, err := os.OpenFile("/etc/security/limits.conf", os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(limitsConf)
		_ = f.Close()
	}

	// 重载sysctl
	_, _ = i.executor.Run(ctx, "sysctl", "-p")
	_, _ = i.executor.Run(ctx, "systemctl", "restart", "systemd-sysctl")

	return nil
}

func (i *installer) installDeps(ctx context.Context, cfg *types.InstallConfig) error {
	info, _ := i.detector.Detect(ctx)
	pkgMgr := system.NewPackageManager(info.OS, i.executor)
	if pkgMgr == nil {
		return errors.New(i18n.T().Get("Unsupported operating system"))
	}

	// 设置镜像源
	if err := pkgMgr.SetMirror(ctx, cfg.InChina); err != nil {
		return err
	}

	// 更新缓存
	if err := pkgMgr.UpdateCache(ctx); err != nil {
		return err
	}

	// 安装EPEL (RHEL系)
	if info.OS == types.OSRHEL {
		_ = pkgMgr.EnableEPEL(ctx, cfg.InChina)
	}

	// 安装依赖
	var packages []string
	if info.OS == types.OSRHEL {
		packages = []string{"bash", "curl", "wget", "zip", "unzip", "tar", "git", "jq", "make", "sudo"}
	} else {
		packages = []string{"bash", "curl", "wget", "zip", "unzip", "tar", "git", "jq", "make", "sudo"}
	}

	return pkgMgr.Install(ctx, packages...)
}

func (i *installer) createSwap(ctx context.Context, cfg *types.InstallConfig) error {
	info, _ := i.detector.Detect(ctx)

	// 如果已有swap或内存>=4G，跳过
	if info.Swap > 1 || info.Memory >= 3900 {
		return nil
	}

	if !cfg.AutoSwap {
		return nil
	}

	swapFile := cfg.SetupPath + "/swap"

	// 创建swap文件
	_, _ = i.executor.Run(ctx, "dd", "if=/dev/zero", "of="+swapFile, "bs=8M", "count=256")
	_, _ = i.executor.Run(ctx, "chmod", "600", swapFile)
	_, _ = i.executor.Run(ctx, "mkswap", "-f", swapFile)
	_, _ = i.executor.Run(ctx, "swapon", swapFile)

	// 添加到fstab
	fstabEntry := fmt.Sprintf("%s    swap    swap    defaults    0 0\n", swapFile)
	f, err := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(fstabEntry)
		_ = f.Close()
	}

	return nil
}

func (i *installer) downloadPanel(ctx context.Context, cfg *types.InstallConfig) error {
	// 创建目录
	_ = os.MkdirAll(cfg.SetupPath+"/panel", 0755)
	_ = os.MkdirAll(cfg.SetupPath+"/server/webhook", 0755)
	_ = os.MkdirAll(cfg.SetupPath+"/server/cron/logs", 0755)
	_ = os.MkdirAll(cfg.SetupPath+"/projects", 0755)

	// 获取最新版本信息
	client := resty.New()
	resp, err := client.R().
		SetContext(ctx).
		Get("https://api.acepanel.net/version/latest")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T().Get("Failed to get version info"), err)
	}

	var versionResp struct {
		Data struct {
			Version   string `json:"version"`
			Downloads []struct {
				Arch     string `json:"arch"`
				URL      string `json:"url"`
				Checksum string `json:"checksum"`
			} `json:"downloads"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body(), &versionResp); err != nil {
		return fmt.Errorf("%s: %w", i18n.T().Get("Failed to parse version info"), err)
	}

	// 根据架构选择下载链接
	info, _ := i.detector.Detect(ctx)
	var downloadURL string
	arch := "amd64"
	if info.Arch == types.ArchARM64 {
		arch = "arm64"
	}

	for _, dl := range versionResp.Data.Downloads {
		if dl.Arch == arch {
			downloadURL = dl.URL
			break
		}
	}

	if downloadURL == "" {
		return errors.New(i18n.T().Get("No download URL found for architecture %s", arch))
	}

	// 下载面板
	zipPath := cfg.SetupPath + "/panel/panel.zip"
	resp, err = client.R().
		SetContext(ctx).
		SetOutput(zipPath).
		Get(downloadURL)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T().Get("Failed to download panel"), err)
	}

	// 解压
	result, err := i.executor.Run(ctx, "unzip", "-o", zipPath, "-d", cfg.SetupPath+"/panel")
	if err != nil || result.ExitCode != 0 {
		return errors.New(i18n.T().Get("Failed to unzip panel"))
	}

	// 删除zip文件
	_ = os.Remove(zipPath)

	// 移动配置文件
	_ = os.Rename(cfg.SetupPath+"/panel/config.example.yml", cfg.SetupPath+"/panel/storage/config.yml")

	// 替换配置中的路径
	_, _ = i.executor.Run(ctx, "sed", "-i", fmt.Sprintf("s|/opt/ace|%s|g", cfg.SetupPath), cfg.SetupPath+"/panel/storage/config.yml")

	// 设置权限
	_, _ = i.executor.Run(ctx, "chmod", "-R", "700", cfg.SetupPath+"/panel")
	_, _ = i.executor.Run(ctx, "chmod", "600", cfg.SetupPath+"/panel/storage/config.yml")

	// 移动CLI工具
	_ = os.Rename(cfg.SetupPath+"/panel/cli", "/usr/local/sbin/acepanel")
	_, _ = i.executor.Run(ctx, "chmod", "+x", "/usr/local/sbin/acepanel")

	return nil
}

func (i *installer) configureFirewall(ctx context.Context, cfg *types.InstallConfig) error {
	// 安装firewalld
	if err := i.firewall.Install(ctx); err != nil {
		return err
	}

	// 启用
	if err := i.firewall.Enable(ctx); err != nil {
		return err
	}

	// 获取SSH端口
	info, _ := i.detector.Detect(ctx)

	// 添加端口
	ports := []struct {
		port     int
		protocol string
	}{
		{22, "tcp"},
		{80, "tcp"},
		{443, "tcp"},
		{443, "udp"},
		{info.SSHPort, "tcp"},
	}

	for _, p := range ports {
		_ = i.firewall.AddPort(ctx, p.port, p.protocol)
	}

	// 重载
	return i.firewall.Reload(ctx)
}

func (i *installer) createService(ctx context.Context, cfg *types.InstallConfig) error {
	serviceContent := fmt.Sprintf(`[Unit]
Description=AcePanel
After=network.target

[Service]
Type=simple
ExecStart=%s/panel/ace
WorkingDirectory=%s/panel
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, cfg.SetupPath, cfg.SetupPath)

	if err := i.systemd.WriteServiceFile("acepanel", serviceContent); err != nil {
		return err
	}

	if err := i.systemd.DaemonReload(ctx); err != nil {
		return err
	}

	return i.systemd.Enable(ctx, "acepanel")
}

func (i *installer) initPanel(ctx context.Context, cfg *types.InstallConfig) error {
	// 初始化面板
	result, err := i.executor.Run(ctx, "/usr/local/sbin/acepanel", "init")
	if err != nil || result.ExitCode != 0 {
		return errors.New(i18n.T().Get("Failed to initialize panel"))
	}

	// 同步
	result, err = i.executor.Run(ctx, "/usr/local/sbin/acepanel", "sync")
	if err != nil || result.ExitCode != 0 {
		return errors.New(i18n.T().Get("Failed to sync panel"))
	}

	return nil
}

func (i *installer) detectApps(ctx context.Context, cfg *types.InstallConfig) error {
	// 检测Docker
	result, _ := i.executor.Run(ctx, "which", "docker")
	if result != nil && result.ExitCode == 0 {
		_, _ = i.executor.Run(ctx, "systemctl", "enable", "--now", "docker")
		versionResult, _ := i.executor.Run(ctx, "docker", "-v")
		if versionResult != nil {
			parts := strings.Fields(versionResult.Stdout)
			if len(parts) >= 3 {
				version := strings.TrimSuffix(parts[2], ",")
				_, _ = i.executor.Run(ctx, "/usr/local/sbin/acepanel", "app", "write", "docker", "stable", version)
			}
		}
	}

	// 检测Podman
	result, _ = i.executor.Run(ctx, "which", "podman")
	if result != nil && result.ExitCode == 0 {
		_, _ = i.executor.Run(ctx, "systemctl", "enable", "--now", "podman")
		_, _ = i.executor.Run(ctx, "systemctl", "enable", "--now", "podman.socket")
		versionResult, _ := i.executor.Run(ctx, "podman", "-v")
		if versionResult != nil {
			parts := strings.Fields(versionResult.Stdout)
			if len(parts) >= 3 {
				_, _ = i.executor.Run(ctx, "/usr/local/sbin/acepanel", "app", "write", "podman", "stable", parts[2])
			}
		}
	}

	// 检测Fail2ban
	result, _ = i.executor.Run(ctx, "which", "fail2ban-server")
	if result != nil && result.ExitCode == 0 {
		_, _ = i.executor.Run(ctx, "systemctl", "enable", "--now", "fail2ban")
		versionResult, _ := i.executor.Run(ctx, "fail2ban-server", "-V")
		if versionResult != nil {
			_, _ = i.executor.Run(ctx, "/usr/local/sbin/acepanel", "app", "write", "fail2ban", "stable", strings.TrimSpace(versionResult.Stdout))
		}
	}

	return nil
}
