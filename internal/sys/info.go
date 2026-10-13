package sys

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Family 发行版家族，决定用 dnf 还是 apt
type Family string

const (
	FamilyRHEL   Family = "rhel"
	FamilyDebian Family = "debian"
)

// Info 系统信息
type Info struct {
	Family   Family
	ID       string // os-release 的 ID，如 almalinux、ubuntu
	Name     string // os-release 的 PRETTY_NAME
	Version  string // 带小版本的版本号，如 9.8、13.7、24.04.5
	Major    int
	Arch     string // amd64 / arm64
	Kernel   string
	MemoryMB int64
	SwapMB   int64
	SSHPort  int
	InChina  bool
}

// Detect 探测系统信息，判断地区要访问网络，所以只探测一次并缓存
var Detect = sync.OnceValue(detect)

func detect() *Info {
	rel := osRelease()
	info := &Info{
		Family:  family(rel["ID"], rel["ID_LIKE"]),
		ID:      rel["ID"],
		Name:    rel["PRETTY_NAME"],
		Version: rel["VERSION_ID"],
		Arch:    runtime.GOARCH,
		SSHPort: sshPort(),
		InChina: inChina(),
	}
	info.Major, _ = strconv.Atoi(strings.SplitN(info.Version, ".", 2)[0])
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		info.Kernel = strings.TrimSpace(string(b))
	}
	// Debian 的 VERSION_ID 只有大版本，小版本在 /etc/debian_version；Ubuntu 的小版本在 VERSION 开头
	switch info.ID {
	case "debian":
		if b, err := os.ReadFile("/etc/debian_version"); err == nil {
			info.Version = strings.TrimSpace(string(b))
		}
	case "ubuntu":
		if f := strings.Fields(rel["VERSION"]); len(f) > 0 {
			info.Version = f[0]
		}
	}
	info.MemoryMB, info.SwapMB = memInfo()
	return info
}

func osRelease() map[string]string {
	rel := map[string]string{}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return rel
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			rel[k] = strings.Trim(v, `"`)
		}
	}
	return rel
}

func family(id, like string) Family {
	for _, name := range strings.Fields(id + " " + like) {
		switch name {
		case "debian", "ubuntu":
			return FamilyDebian
		case "rhel", "centos", "fedora", "rocky", "almalinux", "tencentos", "opencloudos":
			return FamilyRHEL
		}
	}
	return ""
}

func memInfo() (memory, swap int64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kb, _ := strconv.ParseInt(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			memory = kb / 1024
		case "SwapTotal:":
			swap = kb / 1024
		}
	}
	return memory, swap
}

// sshPort 读 sshd 配置里的端口，Include 的 .d 目录优先于主配置
func sshPort() int {
	files, _ := filepath.Glob("/etc/ssh/sshd_config.d/*.conf")
	for _, f := range append(files, "/etc/ssh/sshd_config") {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.EqualFold(fields[0], "Port") {
				if port, err := strconv.Atoi(fields[1]); err == nil {
					return port
				}
			}
		}
	}
	return 22
}

func inChina() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://perfops.cloudflareperf.com/cdn-cgi/trace", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if scanner.Text() == "loc=CN" {
			return true
		}
	}
	return false
}
