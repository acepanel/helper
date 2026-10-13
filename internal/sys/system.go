package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
)

// Systemctl 执行 systemctl 子命令
func Systemctl(ctx context.Context, args ...string) error {
	_, err := Run(ctx, "systemctl", args...)
	return err
}

func unitPath(name string) string {
	return "/etc/systemd/system/" + name + ".service"
}

// WriteUnit 写入服务单元文件
func WriteUnit(name, content string) error {
	return os.WriteFile(unitPath(name), []byte(content), 0o644)
}

// RemoveUnit 删除服务单元文件
func RemoveUnit(name string) error {
	if err := os.Remove(unitPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// EnsureUser 确保用户和组存在，用户不允许登录
func EnsureUser(ctx context.Context, name, group string) error {
	if _, err := user.LookupGroup(group); err != nil {
		if _, err = Run(ctx, "groupadd", group); err != nil {
			return err
		}
	}
	if _, err := user.Lookup(name); err != nil {
		if _, err = Run(ctx, "useradd", "-g", group, "-s", "/sbin/nologin", name); err != nil {
			return err
		}
	}
	return nil
}

// Port 防火墙放行端口
type Port struct {
	Number int
	Proto  string
}

func (p Port) String() string {
	return fmt.Sprintf("%d/%s", p.Number, p.Proto)
}

// SetupFirewall 安装并启用防火墙后放行端口，Debian 系用 ufw，RHEL 系用 firewalld
func SetupFirewall(ctx context.Context, info *Info, ports []Port) error {
	if info.Family == FamilyDebian {
		if err := NewPkg(info).Install(ctx, "ufw"); err != nil {
			return err
		}
		// 先加规则再启用，避免启用瞬间把 SSH 挡在外面
		for _, p := range ports {
			if _, err := Run(ctx, "ufw", "allow", p.String()); err != nil {
				return err
			}
		}
		_, err := Run(ctx, "ufw", "--force", "enable")
		return err
	}

	if err := NewPkg(info).Install(ctx, "firewalld"); err != nil {
		return err
	}
	if err := Systemctl(ctx, "enable", "--now", "firewalld"); err != nil {
		return err
	}
	_, _ = Run(ctx, "firewall-cmd", "--set-default-zone=public")
	for _, p := range ports {
		if _, err := Run(ctx, "firewall-cmd", "--permanent", "--zone=public", "--add-port="+p.String()); err != nil {
			return err
		}
	}
	_, err := Run(ctx, "firewall-cmd", "--reload")
	return err
}
