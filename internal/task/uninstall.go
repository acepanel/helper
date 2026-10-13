package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/pkg/i18n"
)

// UninstallSteps 卸载面板
func UninstallSteps(setupPath string) []Step {
	return []Step{
		{Name: i18n.T.Get("Checking installation"), Run: func(context.Context) error {
			if os.Geteuid() != 0 {
				return errors.New(i18n.T.Get("Please run with root privileges"))
			}
			if _, err := os.Stat(filepath.Join(setupPath, "panel", "ace")); err != nil {
				return errors.New(i18n.T.Get("Panel is not installed"))
			}
			return nil
		}},
		{Name: i18n.T.Get("Stopping panel service"), Run: func(ctx context.Context) error {
			_ = sys.Systemctl(ctx, "disable", "--now", "acepanel")
			if err := sys.RemoveUnit("acepanel"); err != nil {
				return err
			}
			return sys.Systemctl(ctx, "daemon-reload")
		}},
		{Name: i18n.T.Get("Removing CLI tool"), Run: func(context.Context) error {
			for _, f := range []string{cliPath, "/usr/local/sbin/ace"} {
				if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			return nil
		}},
		{Name: i18n.T.Get("Removing swap file"), Run: func(ctx context.Context) error {
			return sys.RemoveSwap(ctx, filepath.Join(setupPath, "swap"))
		}},
		{Name: i18n.T.Get("Removing installation directory"), Run: func(context.Context) error {
			return os.RemoveAll(setupPath)
		}},
	}
}
