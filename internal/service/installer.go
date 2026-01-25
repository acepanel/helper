package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	SetVerboseCallback(cb system.VerboseCallback)
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

func (i *installer) SetVerboseCallback(cb system.VerboseCallback) {
	i.executor.SetVerboseCallback(cb)
}

func (i *installer) Install(ctx context.Context, cfg *types.InstallConfig, progress chan<- types.Progress) error {
	steps := []struct {
		name   string
		weight float64
		fn     func(ctx context.Context, cfg *types.InstallConfig) error
	}{
		{i18n.T.Get("Checking system requirements"), 0.05, i.checkSystem},
		{i18n.T.Get("Creating www user"), 0.02, i.createUser},
		{i18n.T.Get("Optimizing system settings"), 0.08, i.optimizeSystem},
		{i18n.T.Get("Installing dependencies"), 0.20, i.installDeps},
		{i18n.T.Get("Creating swap file"), 0.05, i.createSwap},
		{i18n.T.Get("Downloading panel"), 0.30, i.downloadPanel},
		{i18n.T.Get("Configuring firewall"), 0.10, i.configureFirewall},
		{i18n.T.Get("Creating systemd service"), 0.10, i.createService},
		{i18n.T.Get("Initializing panel"), 0.08, i.initPanel},
		{i18n.T.Get("Detecting installed apps"), 0.02, i.detectApps},
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
				Message: fmt.Sprintf("%s: %v", i18n.T.Get("Error"), err),
				IsError: true,
				Error:   err,
			}
			return err
		}

		currentProgress += step.weight
		progress <- types.Progress{
			Step:    step.name,
			Percent: currentProgress,
			Message: step.name + " - " + i18n.T.Get("completed"),
		}
	}

	progress <- types.Progress{
		Step:    i18n.T.Get("Installation complete"),
		Percent: 1.0,
		Message: i18n.T.Get("Panel installed successfully"),
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
		return errors.New(i18n.T.Get("Unsupported operating system"))
	}
	// 检查架构
	if info.Arch == types.ArchUnknown {
		return errors.New(i18n.T.Get("Unsupported CPU architecture"))
	}

	// 检查CPU特性
	if err = i.detector.CheckCPUFeatures(ctx); err != nil {
		return err
	}

	// 检查内核版本
	if info.KernelVersion != "" {
		parts := strings.Split(info.KernelVersion, ".")
		if len(parts) > 0 {
			major := 0
			_, _ = fmt.Sscanf(parts[0], "%d", &major)
			if major < 5 {
				return errors.New(i18n.T.Get("Kernel version too old, requires 5.x or above"))
			}
		}
	}

	// 检查是否64位
	if !info.Is64Bit {
		return errors.New(i18n.T.Get("Requires 64-bit system"))
	}

	// 检查是否已安装
	if i.detector.CheckPanelInstalled(cfg.SetupPath) {
		return errors.New(i18n.T.Get("Panel is already installed"))
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

	// 临时设置系统参数
	_, _ = i.executor.Run(ctx, "sysctl", "-w", "vm.overcommit_memory=1")
	_, _ = i.executor.Run(ctx, "sysctl", "-w", "net.core.somaxconn=1024")

	// 设置 file-max
	_ = os.WriteFile("/proc/sys/fs/file-max", []byte("2147483584"), 0644)

	// 检查并追加 limits.conf 配置
	limitsContent, _ := os.ReadFile("/etc/security/limits.conf")
	limitsStr := string(limitsContent)
	limitsFile, err := os.OpenFile("/etc/security/limits.conf", os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		if !strings.Contains(limitsStr, "* soft nofile") {
			_, _ = limitsFile.WriteString("* soft nofile 1048576\n")
		}
		if !strings.Contains(limitsStr, "* hard nofile") {
			_, _ = limitsFile.WriteString("* hard nofile 1048576\n")
		}
		if !strings.Contains(limitsStr, "* soft nproc") {
			_, _ = limitsFile.WriteString("* soft nproc 1048576\n")
		}
		if !strings.Contains(limitsStr, "* hard nproc") {
			_, _ = limitsFile.WriteString("* hard nproc 1048576\n")
		}
		_ = limitsFile.Close()
	}

	// 检查 sysctl.conf 中是否已有 fs.file-max
	sysctlContent, _ := os.ReadFile("/etc/sysctl.conf")

	// 构建 sysctl 配置
	var sysctlConf strings.Builder

	// fs.file-max
	if !strings.Contains(string(sysctlContent), "fs.file-max") {
		sysctlConf.WriteString("fs.file-max = 2147483584\n")
	}

	// 自动开启 BBR
	// 清理旧配置
	_, _ = i.executor.Run(ctx, "sed", "-i", "/net.core.default_qdisc/d", "/etc/sysctl.conf")
	_, _ = i.executor.Run(ctx, "sed", "-i", "/net.ipv4.tcp_congestion_control/d", "/etc/sysctl.conf")

	// 检查 BBR 支持
	bbrSupport, _ := i.executor.Run(ctx, "sh", "-c", "ls -l /lib/modules/*/kernel/net/ipv4 2>/dev/null | grep -c tcp_bbr || echo 0")
	bbrOpen, _ := i.executor.Run(ctx, "sh", "-c", "sysctl net.ipv4.tcp_congestion_control 2>/dev/null | grep -c bbr || echo 0")

	bbrSupportCount := strings.TrimSpace(bbrSupport.Stdout)
	bbrOpenCount := strings.TrimSpace(bbrOpen.Stdout)

	if bbrSupportCount != "0" && bbrOpenCount == "0" {
		// 选择最佳 qdisc
		qdisc := ""
		kernelVersion, _ := i.executor.Run(ctx, "uname", "-r")
		if kernelVersion != nil {
			kv := strings.TrimSpace(kernelVersion.Stdout)
			bootConfig := "/boot/config-" + kv

			// 按优先级检查: cake > fq_codel > fq_pie > fq
			if cakeCheck, _ := i.executor.Run(ctx, "sh", "-c", "cat "+bootConfig+" 2>/dev/null | grep CONFIG_NET_SCH_CAKE | grep -q '=' && echo yes"); cakeCheck != nil && strings.TrimSpace(cakeCheck.Stdout) == "yes" {
				qdisc = "cake"
			} else if fqCodelCheck, _ := i.executor.Run(ctx, "sh", "-c", "cat "+bootConfig+" 2>/dev/null | grep CONFIG_NET_SCH_FQ_CODEL | grep -q '=' && echo yes"); fqCodelCheck != nil && strings.TrimSpace(fqCodelCheck.Stdout) == "yes" {
				qdisc = "fq_codel"
			} else if fqPieCheck, _ := i.executor.Run(ctx, "sh", "-c", "cat "+bootConfig+" 2>/dev/null | grep CONFIG_NET_SCH_FQ_PIE | grep -q '=' && echo yes"); fqPieCheck != nil && strings.TrimSpace(fqPieCheck.Stdout) == "yes" {
				qdisc = "fq_pie"
			} else if fqCheck, _ := i.executor.Run(ctx, "sh", "-c", "cat "+bootConfig+" 2>/dev/null | grep CONFIG_NET_SCH_FQ | grep -q '=' && echo yes"); fqCheck != nil && strings.TrimSpace(fqCheck.Stdout) == "yes" {
				qdisc = "fq"
			} else {
				// 获取当前 qdisc
				currentQdisc, _ := i.executor.Run(ctx, "sh", "-c", "sysctl net.core.default_qdisc 2>/dev/null | awk '{print $3}'")
				if currentQdisc != nil {
					qdisc = strings.TrimSpace(currentQdisc.Stdout)
				}
			}
		}
		if qdisc != "" {
			sysctlConf.WriteString(fmt.Sprintf("net.core.default_qdisc=%s\n", qdisc))
		}
		sysctlConf.WriteString("net.ipv4.tcp_congestion_control=bbr\n")
	}

	// nf_conntrack 调优
	// 清理旧配置
	_, _ = i.executor.Run(ctx, "sed", "-i", "/nf_conntrack_max/d", "/etc/sysctl.conf")
	_, _ = i.executor.Run(ctx, "sed", "-i", "/nf_conntrack_buckets/d", "/etc/sysctl.conf")

	info, _ := i.detector.Detect(ctx)
	mem := info.Memory // MB
	if mem < 2100 {
		sysctlConf.WriteString("net.netfilter.nf_conntrack_max=262144\n")
		sysctlConf.WriteString("net.netfilter.nf_conntrack_buckets=65536\n")
	} else if mem < 4100 {
		sysctlConf.WriteString("net.netfilter.nf_conntrack_max=655360\n")
		sysctlConf.WriteString("net.netfilter.nf_conntrack_buckets=163840\n")
	} else if mem < 8200 {
		sysctlConf.WriteString("net.netfilter.nf_conntrack_max=1048576\n")
		sysctlConf.WriteString("net.netfilter.nf_conntrack_buckets=262144\n")
	} else {
		sysctlConf.WriteString("net.netfilter.nf_conntrack_max=1503232\n")
		sysctlConf.WriteString("net.netfilter.nf_conntrack_buckets=375808\n")
	}

	// somaxconn 调优
	_, _ = i.executor.Run(ctx, "sed", "-i", "/net.core.somaxconn/d", "/etc/sysctl.conf")
	sysctlConf.WriteString("net.core.somaxconn=1024\n")

	// 写入 sysctl 配置
	_ = os.WriteFile("/etc/sysctl.d/99-panel.conf", []byte(sysctlConf.String()), 0644)

	// sudoers 添加 /usr/local/bin 和 /usr/local/sbin 路径
	// 检查是否已包含
	sudoersCheck, _ := i.executor.Run(ctx, "sh", "-c", "grep -q '^Defaults.*secure_path.*:/usr/local/bin' /etc/sudoers && echo yes")
	if sudoersCheck == nil || strings.TrimSpace(sudoersCheck.Stdout) != "yes" {
		_, _ = i.executor.Run(ctx, "sed", "-i",
			`s|^\(Defaults\s*secure_path\s*=\s*/sbin:/bin:/usr/sbin:/usr/bin\)$|\1:/usr/local/sbin:/usr/local/bin|`,
			"/etc/sudoers")
	}

	// 重载 sysctl
	_, _ = i.executor.Run(ctx, "sysctl", "-p")
	_, _ = i.executor.Run(ctx, "systemctl", "restart", "systemd-sysctl")

	return nil
}

func (i *installer) installDeps(ctx context.Context, cfg *types.InstallConfig) error {
	info, _ := i.detector.Detect(ctx)
	pkgMgr := system.NewPackageManager(info.OS, i.executor)
	if pkgMgr == nil {
		return errors.New(i18n.T.Get("Unsupported operating system"))
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
		packages = []string{"sudo", "bash", "curl", "wget", "aria2", "zip", "unzip", "tar", "p7zip", "p7zip-plugins", "git", "jq", "dos2unix", "make"}
	} else {
		packages = []string{"sudo", "bash", "curl", "wget", "aria2", "zip", "unzip", "tar", "p7zip", "p7zip-full", "git", "jq", "dos2unix", "make"}
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

	// 检查是否是 btrfs 文件系统
	btrfsCheck, _ := i.executor.Run(ctx, "sh", "-c", fmt.Sprintf("df -T %s | awk '{print $2}' | tail -n 1", cfg.SetupPath))
	if btrfsCheck != nil && strings.TrimSpace(btrfsCheck.Stdout) == "btrfs" {
		// btrfs 文件系统使用专用命令创建 swap
		_, _ = i.executor.Run(ctx, "btrfs", "filesystem", "mkswapfile", "--size", "2G", "--uuid", "clear", swapFile)
	} else {
		// 普通文件系统使用 dd
		_, _ = i.executor.Run(ctx, "dd", "if=/dev/zero", "of="+swapFile, "bs=8M", "count=256")
	}

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

	// 验证 fstab 配置
	result, _ := i.executor.Run(ctx, "mount", "-a")
	if result != nil && result.ExitCode != 0 {
		return errors.New(i18n.T.Get("There is an error in the /etc/fstab file configuration"))
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
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to get version info"), err)
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

	if err = json.Unmarshal(resp.Body(), &versionResp); err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to parse version info"), err)
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
		return errors.New(i18n.T.Get("No download URL found for architecture %s", arch))
	}

	// 下载面板
	zipPath := cfg.SetupPath + "/panel/panel.zip"
	_, err = client.R().
		SetContext(ctx).
		SetOutput(zipPath).
		Get("https://dl.acepanel.net" + downloadURL)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to download panel"), err)
	}

	// 校验SHA256
	resp, err = client.R().
		SetContext(ctx).
		Get("https://dl.acepanel.net" + downloadURL + ".sha256")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to download checksum"), err)
	}
	expectedHash := strings.TrimSpace(strings.Split(string(resp.Body()), " ")[0])

	f, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to open downloaded file"), err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to compute checksum"), err)
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	if actualHash != expectedHash {
		return errors.New(i18n.T.Get("Checksum mismatch"))
	}

	// 解压
	result, err := i.executor.Run(ctx, "unzip", "-o", zipPath, "-d", cfg.SetupPath+"/panel")
	if err != nil || result.ExitCode != 0 {
		return errors.New(i18n.T.Get("Failed to unzip panel"))
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
After=syslog.target network.target
Wants=network.target

[Service]
Type=simple
ExecStart=%s/panel/ace
WorkingDirectory=%s/panel
User=root
Restart=always
RestartSec=5
LimitNOFILE=1048576
LimitNPROC=1048576
Delegate=yes

[Install]
WantedBy=multi-user.target
`, cfg.SetupPath, cfg.SetupPath)

	if err := i.systemd.WriteServiceFile("acepanel", serviceContent); err != nil {
		return err
	}

	if err := i.systemd.DaemonReload(ctx); err != nil {
		return err
	}

	return nil
}

func (i *installer) initPanel(ctx context.Context, cfg *types.InstallConfig) error {
	// 初始化面板
	result, err := i.executor.Run(ctx, "/usr/local/sbin/acepanel", "init")
	if err != nil || result.ExitCode != 0 {
		return errors.New(i18n.T.Get("Failed to initialize panel"))
	}

	// 同步
	result, err = i.executor.Run(ctx, "/usr/local/sbin/acepanel", "sync")
	if err != nil || result.ExitCode != 0 {
		return errors.New(i18n.T.Get("Failed to sync panel"))
	}

	return i.systemd.Enable(ctx, "acepanel")
}

func (i *installer) detectApps(ctx context.Context, cfg *types.InstallConfig) error {
	dockerFound := false

	// 检测Docker
	result, _ := i.executor.Run(ctx, "which", "docker")
	if result != nil && result.ExitCode == 0 {
		dockerFound = true
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
	if result != nil && result.ExitCode == 0 && !dockerFound {
		_, _ = i.executor.Run(ctx, "systemctl", "enable", "--now", "podman")
		_, _ = i.executor.Run(ctx, "systemctl", "enable", "--now", "podman.socket")
		_, _ = i.executor.Run(ctx, "systemctl", "enable", "--now", "podman-restart")
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
