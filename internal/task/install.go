package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/pkg/i18n"
)

const (
	versionAPI   = "https://api.acepanel.net/version/latest"
	downloadHost = "https://dl.acepanel.net"
	cliPath      = "/usr/local/sbin/acepanel"
)

// InstallSteps 安装面板的全部步骤，依赖 Preflight 的检测结果，面板信息写入 panelInfo
func InstallSteps(setupPath string, info *sys.Info, panelInfo *string) []Step {
	pkg := sys.NewPkg(info)
	return []Step{
		{Name: i18n.T.Get("Creating www user"), Run: func(ctx context.Context) error {
			return sys.EnsureUser(ctx, "www", "www")
		}},
		{Name: i18n.T.Get("Installing dependencies"), Run: func(ctx context.Context) error {
			return installDeps(ctx, pkg, info)
		}},
		{Name: i18n.T.Get("Optimizing system settings"), Run: func(ctx context.Context) error {
			return optimizeSystem(ctx, info)
		}},
		{Name: i18n.T.Get("Creating swap file"), Optional: true, Run: func(ctx context.Context) error {
			// 内存不足 4G 且没有 swap 时才创建
			if info.SwapMB > 1 || info.MemoryMB >= 3900 {
				return nil
			}
			return sys.CreateSwap(ctx, filepath.Join(setupPath, "swap"), 2048)
		}},
		{Name: i18n.T.Get("Downloading panel"), Run: func(ctx context.Context) error {
			return downloadPanel(ctx, setupPath, info)
		}},
		{Name: i18n.T.Get("Configuring firewall"), Optional: true, Run: func(ctx context.Context) error {
			return sys.SetupFirewall(ctx, info, []sys.Port{
				{Number: 22, Proto: "tcp"},
				{Number: 80, Proto: "tcp"},
				{Number: 443, Proto: "tcp"},
				{Number: 443, Proto: "udp"},
				{Number: info.SSHPort, Proto: "tcp"},
			})
		}},
		{Name: i18n.T.Get("Creating systemd service"), Run: func(ctx context.Context) error {
			return createService(ctx, setupPath)
		}},
		{Name: i18n.T.Get("Initializing panel"), Run: initPanel},
		{Name: i18n.T.Get("Detecting installed apps"), Optional: true, Run: detectApps},
		{Name: i18n.T.Get("Collecting panel information"), Run: func(ctx context.Context) (err error) {
			*panelInfo, err = sys.Run(ctx, cliPath, "info")
			*panelInfo = strings.TrimSpace(*panelInfo)
			return err
		}},
	}
}

func installDeps(ctx context.Context, pkg *sys.Pkg, info *sys.Info) error {
	if err := pkg.EnableEPEL(ctx); err != nil {
		return err
	}
	pkgs := []string{"sudo", "bash", "curl", "wget", "aria2", "zip", "unzip", "tar", "zstd", "bzip2", "7zip", "git", "jq", "dos2unix", "make"}
	if info.Family == sys.FamilyRHEL {
		pkgs = append(pkgs, "xz")
	} else {
		pkgs = append(pkgs, "xz-utils")
	}
	if err := pkg.Install(ctx, pkgs...); err != nil {
		return err
	}
	return link7z()
}

// link7z Ubuntu 22.04 和 Debian 12 的 7zip 包只带 7zz，补一个 7z 软链给安装脚本用
func link7z() error {
	if _, err := exec.LookPath("7z"); err == nil {
		return nil
	}
	zz, err := exec.LookPath("7zz")
	if err != nil {
		return err
	}
	_ = os.Remove("/usr/local/bin/7z")
	return os.Symlink(zz, "/usr/local/bin/7z")
}

func optimizeSystem(ctx context.Context, info *sys.Info) error {
	if info.InChina {
		_, _ = sys.Run(ctx, "timedatectl", "set-timezone", "Asia/Shanghai")
	}
	if info.Family == sys.FamilyRHEL {
		_, _ = sys.Run(ctx, "setenforce", "0")
		_ = sys.RewriteLines("/etc/selinux/config", func(line string) (string, bool) {
			return strings.Replace(line, "SELINUX=enforcing", "SELINUX=disabled", 1), true
		})
	}
	if err := sys.AppendMissing("/etc/security/limits.conf", [][2]string{
		{"* soft nofile", "* soft nofile 1048576"},
		{"* hard nofile", "* hard nofile 1048576"},
		{"* soft nproc", "* soft nproc 1048576"},
		{"* hard nproc", "* hard nproc 1048576"},
	}); err != nil {
		return err
	}
	if err := writeSysctl(ctx, info); err != nil {
		return err
	}
	if err := fixSudoers(ctx); err != nil {
		return err
	}
	// /etc/environment 没有 PATH 时补一个带 /usr/local 的
	return sys.AppendMissing("/etc/environment", [][2]string{
		{"PATH=", `PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"`},
	})
}

func writeSysctl(ctx context.Context, info *sys.Info) error {
	settings := [][2]string{
		{"fs.file-max", "2147483584"},
		{"vm.overcommit_memory", "1"},
		{"vm.swappiness", "10"},
		{"net.core.netdev_max_backlog", "16384"},
		{"net.core.somaxconn", "8192"},
		{"net.ipv4.tcp_max_syn_backlog", "8192"},
	}
	// 连接跟踪表按内存分档
	switch mem := info.MemoryMB; {
	case mem < 2100:
		settings = append(settings, [2]string{"net.netfilter.nf_conntrack_max", "262144"}, [2]string{"net.netfilter.nf_conntrack_buckets", "65536"})
	case mem < 4100:
		settings = append(settings, [2]string{"net.netfilter.nf_conntrack_max", "655360"}, [2]string{"net.netfilter.nf_conntrack_buckets", "163840"})
	case mem < 8200:
		settings = append(settings, [2]string{"net.netfilter.nf_conntrack_max", "1048576"}, [2]string{"net.netfilter.nf_conntrack_buckets", "262144"})
	default:
		settings = append(settings, [2]string{"net.netfilter.nf_conntrack_max", "1503232"}, [2]string{"net.netfilter.nf_conntrack_buckets", "375808"})
	}
	// BBR 要内核支持，fq 是它推荐搭配的队列
	_, _ = sys.Run(ctx, "modprobe", "tcp_bbr")
	if avail, _ := os.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control"); strings.Contains(string(avail), "bbr") {
		if _, err := sys.Run(ctx, "modprobe", "sch_fq"); err == nil {
			settings = append(settings, [2]string{"net.core.default_qdisc", "fq"})
		}
		settings = append(settings, [2]string{"net.ipv4.tcp_congestion_control", "bbr"})
	}

	// 这些键统一由 99-acepanel.conf 管理，从 sysctl.conf 里清掉避免覆盖
	err := sys.RewriteLines("/etc/sysctl.conf", func(line string) (string, bool) {
		key := strings.TrimSpace(strings.SplitN(line, "=", 2)[0])
		return line, !slices.ContainsFunc(settings, func(s [2]string) bool { return s[0] == key })
	})
	if err != nil {
		return err
	}
	var conf strings.Builder
	for _, s := range settings {
		_, _ = fmt.Fprintf(&conf, "%s = %s\n", s[0], s[1])
	}
	if err = os.MkdirAll("/etc/sysctl.d", 0o755); err != nil {
		return err
	}
	if err = os.WriteFile("/etc/sysctl.d/99-acepanel.conf", []byte(conf.String()), 0o644); err != nil {
		return err
	}
	_, _ = sys.Run(ctx, "modprobe", "nf_conntrack")
	_, _ = sys.Run(ctx, "sysctl", "--system")
	return nil
}

// fixSudoers 通过 sudoers.d 补上 /usr/local 路径，RHEL 系默认的 secure_path 没有它们
func fixSudoers(ctx context.Context) error {
	data, err := os.ReadFile("/etc/sudoers")
	if err != nil {
		return nil // 没装 sudo
	}
	current := "/usr/sbin:/usr/bin:/sbin:/bin"
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "Defaults" && strings.Contains(line, "secure_path") {
			_, value, _ := strings.Cut(line, "=")
			value, _, _ = strings.Cut(value, "#")
			current = strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	dirs := strings.Split(current, ":")
	changed := false
	for _, d := range []string{"/usr/local/sbin", "/usr/local/bin"} {
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	path := "/etc/sudoers.d/acepanel"
	if err = os.WriteFile(path, []byte(fmt.Sprintf("Defaults secure_path=\"%s\"\n", strings.Join(dirs, ":"))), 0o440); err != nil {
		return err
	}
	if _, err = sys.Run(ctx, "visudo", "-cf", path); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func downloadPanel(ctx context.Context, setupPath string, info *sys.Info) error {
	for _, dir := range []string{"panel", "server/webhook", "server/cron/logs", "projects"} {
		if err := os.MkdirAll(filepath.Join(setupPath, dir), 0o755); err != nil {
			return err
		}
	}

	var latest struct {
		Data struct {
			Downloads []struct {
				Arch     string `json:"arch"`
				URL      string `json:"url"`
				Checksum string `json:"checksum"`
			} `json:"downloads"`
		} `json:"data"`
	}
	body, err := httpGet(ctx, versionAPI)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to get version info"), err)
	}
	if err = json.Unmarshal(body, &latest); err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to parse version info"), err)
	}
	idx := slices.IndexFunc(latest.Data.Downloads, func(d struct {
		Arch     string `json:"arch"`
		URL      string `json:"url"`
		Checksum string `json:"checksum"`
	}) bool {
		return d.Arch == info.Arch
	})
	if idx < 0 {
		return errors.New(i18n.T.Get("No download URL found for architecture %s", info.Arch))
	}
	dl := latest.Data.Downloads[idx]

	panelDir := filepath.Join(setupPath, "panel")
	zipPath := filepath.Join(panelDir, "panel.zip")
	_, err = sys.Run(ctx, "aria2c", "-c", "--file-allocation=falloc", "--allow-overwrite=true", "--auto-file-renaming=false",
		"--console-log-level=notice", "--summary-interval=2", "--retry-wait=5", "--max-tries=5",
		"-x", "16", "-s", "16", "-k", "1M", "-d", panelDir, "-o", filepath.Base(zipPath), downloadHost+dl.URL)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to download panel"), err)
	}

	sum, err := httpGet(ctx, downloadHost+dl.Checksum)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to download checksum"), err)
	}
	if err = verifySHA256(zipPath, string(sum)); err != nil {
		return err
	}
	if _, err = sys.Run(ctx, "unzip", "-o", zipPath, "-d", panelDir); err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to unzip panel"), err)
	}
	_ = os.Remove(zipPath)

	// 配置文件里的默认路径换成实际安装路径
	config := filepath.Join(panelDir, "storage", "config.yml")
	if err = os.Rename(filepath.Join(panelDir, "config.example.yml"), config); err != nil {
		return err
	}
	if err = sys.RewriteLines(config, func(line string) (string, bool) {
		return strings.ReplaceAll(line, "/opt/ace", setupPath), true
	}); err != nil {
		return err
	}
	if _, err = sys.Run(ctx, "chmod", "-R", "700", panelDir); err != nil {
		return err
	}
	if err = os.Chmod(config, 0o600); err != nil {
		return err
	}

	// CLI 装到 /usr/local/sbin，ace 作为短命令软链接
	if _, err = sys.Run(ctx, "mv", "-f", filepath.Join(panelDir, "cli"), cliPath); err != nil {
		return err
	}
	if err = os.Chmod(cliPath, 0o755); err != nil {
		return err
	}
	_ = os.Remove("/usr/local/sbin/ace")
	return os.Symlink(cliPath, "/usr/local/sbin/ace")
}

func verifySHA256(path, expected string) error {
	fields := strings.Fields(expected)
	if len(fields) == 0 {
		return errors.New(i18n.T.Get("Checksum mismatch"))
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != fields[0] {
		return errors.New(i18n.T.Get("Checksum mismatch"))
	}
	return nil
}

func createService(ctx context.Context, setupPath string) error {
	unit := fmt.Sprintf(`[Unit]
Description=AcePanel
After=network.target
Wants=network.target

[Service]
Type=simple
ExecStart=%[1]s/panel/ace
WorkingDirectory=%[1]s/panel
User=root
Restart=always
RestartSec=5
LimitNOFILE=1048576
LimitNPROC=1048576
Delegate=yes

[Install]
WantedBy=multi-user.target
`, setupPath)
	if err := sys.WriteUnit("acepanel", unit); err != nil {
		return err
	}
	return sys.Systemctl(ctx, "daemon-reload")
}

func initPanel(ctx context.Context) error {
	if _, err := sys.Run(ctx, cliPath, "init"); err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to initialize panel"), err)
	}
	if _, err := sys.Run(ctx, cliPath, "sync"); err != nil {
		return fmt.Errorf("%s: %w", i18n.T.Get("Failed to sync panel"), err)
	}
	return sys.Systemctl(ctx, "enable", "--now", "acepanel")
}

// detectApps 把机器上已有的 Docker/Podman/Fail2ban 登记到面板
func detectApps(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err == nil {
		_ = sys.Systemctl(ctx, "enable", "--now", "docker")
		// 输出形如 "Docker version 27.3.1, build xxx"
		if out, err := sys.Run(ctx, "docker", "-v"); err == nil {
			if f := strings.Fields(out); len(f) >= 3 {
				_, _ = sys.Run(ctx, cliPath, "app", "write", "docker", "stable", strings.TrimSuffix(f[2], ","))
			}
		}
	} else if _, err = exec.LookPath("podman"); err == nil {
		for _, unit := range []string{"podman", "podman.socket", "podman-restart"} {
			_ = sys.Systemctl(ctx, "enable", "--now", unit)
		}
		// 输出形如 "podman version 4.9.4"
		if out, err := sys.Run(ctx, "podman", "-v"); err == nil {
			if f := strings.Fields(out); len(f) >= 3 {
				_, _ = sys.Run(ctx, cliPath, "app", "write", "podman", "stable", f[2])
			}
		}
	}
	if _, err := exec.LookPath("fail2ban-server"); err == nil {
		_ = sys.Systemctl(ctx, "enable", "--now", "fail2ban")
		if out, err := sys.Run(ctx, "fail2ban-server", "-V"); err == nil {
			_, _ = sys.Run(ctx, cliPath, "app", "write", "fail2ban", "stable", strings.TrimSpace(out))
		}
	}
	return nil
}
