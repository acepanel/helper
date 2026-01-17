package system

import (
	"context"
	"fmt"

	"github.com/acepanel/helper/pkg/i18n"
)

// UserManager 用户管理接口
type UserManager interface {
	// UserExists 检查用户是否存在
	UserExists(ctx context.Context, username string) bool
	// GroupExists 检查组是否存在
	GroupExists(ctx context.Context, groupname string) bool
	// CreateUser 创建用户
	CreateUser(ctx context.Context, username, groupname string, nologin bool) error
	// CreateGroup 创建组
	CreateGroup(ctx context.Context, groupname string) error
	// EnsureUserAndGroup 确保用户和组存在
	EnsureUserAndGroup(ctx context.Context, username, groupname string) error
}

type userManager struct {
	executor Executor
}

// NewUserManager 创建用户管理器
func NewUserManager(executor Executor) UserManager {
	return &userManager{executor: executor}
}

func (u *userManager) UserExists(ctx context.Context, username string) bool {
	result, _ := u.executor.Run(ctx, "id", "-u", username)
	return result != nil && result.ExitCode == 0
}

func (u *userManager) GroupExists(ctx context.Context, groupname string) bool {
	result, _ := u.executor.Run(ctx, "getent", "group", groupname)
	return result != nil && result.ExitCode == 0
}

func (u *userManager) CreateUser(ctx context.Context, username, groupname string, nologin bool) error {
	args := []string{"-g", groupname}
	if nologin {
		args = append(args, "-s", "/sbin/nologin")
	}
	args = append(args, username)

	result, err := u.executor.Run(ctx, "useradd", args...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T.Get("Failed to create user"), username, result.Stderr)
	}
	return nil
}

func (u *userManager) CreateGroup(ctx context.Context, groupname string) error {
	result, err := u.executor.Run(ctx, "groupadd", groupname)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %s: %s", i18n.T.Get("Failed to create group"), groupname, result.Stderr)
	}
	return nil
}

func (u *userManager) EnsureUserAndGroup(ctx context.Context, username, groupname string) error {
	// 确保组存在
	if !u.GroupExists(ctx, groupname) {
		if err := u.CreateGroup(ctx, groupname); err != nil {
			return err
		}
	}

	// 确保用户存在
	if !u.UserExists(ctx, username) {
		if err := u.CreateUser(ctx, username, groupname, true); err != nil {
			return err
		}
	}

	return nil
}
