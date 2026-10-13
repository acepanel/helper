package sys

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Pkg 包管理器，按发行版家族分派到 dnf 或 apt
type Pkg struct {
	info *Info
}

func NewPkg(info *Info) *Pkg {
	return &Pkg{info: info}
}

// Refresh 刷新软件源缓存
func (p *Pkg) Refresh(ctx context.Context) error {
	if p.info.Family == FamilyRHEL {
		_, err := Run(ctx, "dnf", "-y", "makecache")
		return err
	}
	_, err := Run(ctx, "apt-get", "update")
	return err
}

// Install 安装软件包
func (p *Pkg) Install(ctx context.Context, pkgs ...string) error {
	if p.info.Family == FamilyRHEL {
		// 最小安装自带的 curl-minimal 和 curl 冲突，允许替换
		_, err := Run(ctx, "dnf", append([]string{"-y", "--allowerasing", "install"}, pkgs...)...)
		return err
	}
	_, err := Run(ctx, "apt-get", append([]string{"-y", "install"}, pkgs...)...)
	return err
}

// Upgrade 升级全部软件包，apt 下保留本地改过的配置文件避免交互
func (p *Pkg) Upgrade(ctx context.Context) error {
	if p.info.Family == FamilyRHEL {
		_, err := Run(ctx, "dnf", "-y", "upgrade")
		return err
	}
	_, err := Run(ctx, "apt-get", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "full-upgrade")
	return err
}

// ReleaseUpdate 检查发行版是否有新的小版本：os-release 所属的软件包可升级即视为落后，
// 返回能解析出的目标版本号，解析不出（Ubuntu 的 base-files 版本号对不上小版本）时为空
func (p *Pkg) ReleaseUpdate(ctx context.Context) (outdated bool, latest string, err error) {
	if p.info.Family == FamilyRHEL {
		return dnfReleaseUpdate(ctx)
	}
	return aptReleaseUpdate(ctx)
}

func dnfReleaseUpdate(ctx context.Context) (bool, string, error) {
	name, err := Run(ctx, "rpm", "-qf", "--qf", "%{NAME}", "/etc/os-release")
	if err != nil {
		return false, "", err
	}
	// 退出码 100 表示有更新，输出形如 "almalinux-release.noarch  9.8-1.el9  baseos"
	stdout, stderr, code, err := Exec(ctx, "", "dnf", "-q", "check-update", name)
	if code == 100 {
		for _, line := range strings.Split(stdout, "\n") {
			if fields := strings.Fields(line); len(fields) >= 2 && strings.HasPrefix(fields[0], name+".") {
				return true, strings.SplitN(fields[1], "-", 2)[0], nil
			}
		}
		return true, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("dnf check-update: %w: %s", err, tail(stderr, 5))
	}
	return false, "", nil
}

var debianPoint = regexp.MustCompile(`\+deb(\d+)u(\d+)`)

func aptReleaseUpdate(ctx context.Context) (bool, string, error) {
	out, err := Run(ctx, "apt-cache", "policy", "base-files")
	if err != nil {
		return false, "", err
	}
	var installed, candidate string
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), ": "); ok {
			switch k {
			case "Installed":
				installed = v
			case "Candidate":
				candidate = v
			}
		}
	}
	if candidate == "" || candidate == "(none)" || candidate == installed {
		return false, "", nil
	}
	// Debian 的 base-files 版本 13.8+deb13u7 对应 13.7
	if m := debianPoint.FindStringSubmatch(candidate); m != nil {
		return true, m[1] + "." + m[2], nil
	}
	return true, "", nil
}

var (
	debianSecurity = regexp.MustCompile(`security\.debian\.org/?(debian-security)?`)
	ubuntuHost     = regexp.MustCompile(`(?:[a-z]+\.)?(?:archive|security|ports)\.ubuntu\.com`)
)

// UseMirror 国内机器把软件源切到腾讯云镜像
func (p *Pkg) UseMirror() error {
	if !p.info.InChina {
		return nil
	}
	switch p.info.ID {
	case "debian":
		return rewriteSources([]string{"/etc/apt/sources.list", "/etc/apt/sources.list.d/debian.sources"}, func(line string) string {
			line = debianSecurity.ReplaceAllString(line, "mirrors.tencent.com/debian-security")
			return strings.ReplaceAll(line, "deb.debian.org", "mirrors.tencent.com")
		})
	case "ubuntu":
		return rewriteSources([]string{"/etc/apt/sources.list", "/etc/apt/sources.list.d/ubuntu.sources"}, func(line string) string {
			return ubuntuHost.ReplaceAllString(line, "mirrors.tencent.com")
		})
	case "almalinux":
		return rewriteRepos("/etc/yum.repos.d/almalinux*.repo", "https://repo.almalinux.org", "https://mirrors.tencent.com")
	case "rocky":
		return rewriteRepos("/etc/yum.repos.d/rocky*.repo", "http://dl.rockylinux.org/$contentdir", "https://mirrors.tencent.com/rocky")
	}
	return nil
}

// EnableEPEL RHEL 系启用 EPEL 和 CRB 源
func (p *Pkg) EnableEPEL(ctx context.Context) error {
	if p.info.Family != FamilyRHEL || p.info.ID == "fedora" {
		return nil
	}
	// Rocky 最小安装没有 config-manager 插件；RHEL 官方源里没有 epel-release，退回 Fedora 的 rpm
	if _, err := Run(ctx, "dnf", "-y", "install", "dnf-plugins-core", "epel-release"); err != nil {
		host := "https://dl.fedoraproject.org/pub/epel"
		if p.info.InChina {
			host = "https://mirrors.tencent.com/epel"
		}
		if _, err = Run(ctx, "dnf", "-y", "install", "dnf-plugins-core", fmt.Sprintf("%s/epel-release-latest-%d.noarch.rpm", host, p.info.Major)); err != nil {
			return err
		}
	}
	_, _ = Run(ctx, "dnf", "config-manager", "--set-enabled", "crb")
	if !p.info.InChina {
		return nil
	}
	// openh264 源没有国内镜像，直接删掉
	_ = os.Remove("/etc/yum.repos.d/epel-cisco-openh264.repo")
	return rewriteRepos("/etc/yum.repos.d/epel*.repo", "https://download.example/pub", "https://mirrors.tencent.com")
}

func rewriteSources(files []string, fn func(string) string) error {
	for _, f := range files {
		if err := RewriteLines(f, func(line string) (string, bool) { return fn(line), true }); err != nil {
			return err
		}
	}
	return nil
}

// rewriteRepos 把 repo 文件里注释掉的 baseurl 换成镜像地址并启用，同时注释掉 mirrorlist/metalink
func rewriteRepos(pattern, from, to string) error {
	files, _ := filepath.Glob(pattern)
	for _, f := range files {
		err := RewriteLines(f, func(line string) (string, bool) {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "mirrorlist="), strings.HasPrefix(trimmed, "metalink="):
				return "#" + line, true
			case strings.HasPrefix(trimmed, "#"):
				rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
				if url, ok := strings.CutPrefix(rest, "baseurl="); ok && strings.HasPrefix(url, from) {
					return "baseurl=" + to + strings.TrimPrefix(url, from), true
				}
			}
			return line, true
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// RebootRequired 运行中的内核不是已安装的最新内核，或系统留下了重启标记
func RebootRequired() bool {
	if _, err := os.Stat("/run/reboot-required"); err == nil {
		return true
	}
	running, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	entries, err := os.ReadDir("/lib/modules")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && versionLess(strings.TrimSpace(string(running)), e.Name()) {
			return true
		}
	}
	return false
}

var digits = regexp.MustCompile(`\d+`)

// versionLess 按数字段比较版本字符串
func versionLess(a, b string) bool {
	as, bs := digits.FindAllString(a, -1), digits.FindAllString(b, -1)
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, _ := strconv.Atoi(as[i])
		y, _ := strconv.Atoi(bs[i])
		if x != y {
			return x < y
		}
	}
	return len(as) < len(bs)
}
