package system

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/acepanel/helper/pkg/i18n"
	"github.com/acepanel/helper/pkg/types"
)

// Detector 系统检测器接口
type Detector interface {
	// Detect 检测系统信息
	Detect(ctx context.Context) (*types.SystemInfo, error)
	// CheckRoot 检查root权限
	CheckRoot() error
	// CheckCPUFeatures 检查CPU特性(x86-64-v2)
	CheckCPUFeatures(ctx context.Context) error
	// CheckPanelInstalled 检查面板是否已安装
	CheckPanelInstalled(path string) bool
	// ListDisks 列出可用磁盘
	ListDisks(ctx context.Context) ([]types.DiskInfo, error)
	// CheckDiskExists 检查磁盘是否存在
	CheckDiskExists(disk string) bool
	// IsSystemDisk 检查是否为系统盘
	IsSystemDisk(disk string) bool
}

type detector struct {
	executor Executor
}

// NewDetector 创建检测器
func NewDetector(executor Executor) Detector {
	return &detector{executor: executor}
}

func (d *detector) Detect(ctx context.Context) (*types.SystemInfo, error) {
	info := &types.SystemInfo{}

	// 检测OS类型
	info.OS = d.detectOS()
	// 检测架构
	info.Arch = d.detectArch()
	// 检测内核版本
	info.KernelVersion = d.detectKernelVersion(ctx)
	// 检测是否64位
	info.Is64Bit = d.detect64Bit(ctx)
	// 检测内存
	info.Memory = d.detectMemory(ctx)
	// 检测Swap
	info.Swap = d.detectSwap(ctx)
	// 检测是否在中国
	info.InChina = d.detectInChina(ctx)
	// 检测SSH端口
	info.SSHPort = d.detectSSHPort()

	return info, nil
}

func (d *detector) detectOS() types.OSType {
	// 读取 /etc/os-release
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return types.OSUnknown
	}
	defer func(file *os.File) { _ = file.Close() }(file)

	scanner := bufio.NewScanner(file)
	var id string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			break
		}
	}

	switch id {
	case "debian":
		return types.OSDebian
	case "ubuntu":
		return types.OSUbuntu
	case "rhel", "centos", "rocky", "almalinux", "fedora":
		return types.OSRHEL
	default:
		// 检查ID_LIKE
		_, _ = file.Seek(0, 0)
		scanner = bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "ID_LIKE=") {
				idLike := strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), "\"")
				if strings.Contains(idLike, "debian") {
					return types.OSDebian
				}
				if strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") {
					return types.OSRHEL
				}
			}
		}
	}

	return types.OSUnknown
}

func (d *detector) detectArch() types.ArchType {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return types.ArchAMD64
	case "arm64":
		return types.ArchARM64
	default:
		return types.ArchUnknown
	}
}

func (d *detector) detectKernelVersion(ctx context.Context) string {
	result, err := d.executor.Run(ctx, "uname", "-r")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func (d *detector) detect64Bit(ctx context.Context) bool {
	result, err := d.executor.Run(ctx, "getconf", "LONG_BIT")
	if err != nil {
		return false
	}
	return strings.TrimSpace(result.Stdout) == "64"
}

func (d *detector) detectMemory(ctx context.Context) int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return kb / 1024 // 转换为MB
			}
		}
	}
	return 0
}

func (d *detector) detectSwap(ctx context.Context) int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "SwapTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return kb / 1024 // 转换为MB
			}
		}
	}
	return 0
}

func (d *detector) detectInChina(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://perfops.cloudflareperf.com/cdn-cgi/trace", nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if scanner.Text() == "loc=CN" {
			return true
		}
	}
	return false
}

func (d *detector) detectSSHPort() int {
	file, err := os.Open("/etc/ssh/sshd_config")
	if err != nil {
		return 22
	}
	defer func(file *os.File) { _ = file.Close() }(file)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Port ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				port, err := strconv.Atoi(fields[1])
				if err == nil {
					return port
				}
			}
		}
	}
	return 22
}

func (d *detector) CheckRoot() error {
	currentUser, err := user.Current()
	if err != nil {
		return err
	}
	if currentUser.Uid != "0" {
		return errors.New(i18n.T.Get("Please run with root privileges"))
	}
	return nil
}

func (d *detector) CheckCPUFeatures(ctx context.Context) error {
	// 只有x86_64需要检查
	if runtime.GOARCH != "amd64" {
		return nil
	}

	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return err
	}

	// 检查是否支持ssse3 (x86-64-v2的标志之一)
	if !strings.Contains(string(data), "ssse3") {
		return errors.New(i18n.T.Get("CPU must support at least x86-64-v2 instruction set"))
	}

	return nil
}

func (d *detector) CheckPanelInstalled(path string) bool {
	_, err := os.Stat(path + "/panel/web")
	return err == nil
}

func (d *detector) ListDisks(ctx context.Context) ([]types.DiskInfo, error) {
	result, err := d.executor.Run(ctx, "lsblk", "-dno", "NAME,SIZE,TYPE")
	if err != nil {
		return nil, err
	}

	var disks []types.DiskInfo
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == "disk" {
			// 排除系统盘
			if d.IsSystemDisk(fields[0]) {
				continue
			}
			disks = append(disks, types.DiskInfo{
				Name: fields[0],
				Size: fields[1],
				Type: fields[2],
			})
		}
	}

	return disks, nil
}

func (d *detector) CheckDiskExists(disk string) bool {
	_, err := os.Stat("/dev/" + disk)
	return err == nil
}

func (d *detector) IsSystemDisk(disk string) bool {
	// 系统盘通常以a结尾 (sda, vda, nvme0n1)
	matched, _ := regexp.MatchString(`^(sd|vd|hd)a$`, disk)
	if matched {
		return true
	}
	matched, _ = regexp.MatchString(`^nvme0n1$`, disk)
	return matched
}
