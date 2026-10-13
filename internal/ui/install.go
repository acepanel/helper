package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/internal/task"
	"github.com/acepanel/helper/pkg/i18n"
)

type installState int

const (
	installConfirm installState = iota
	installPreflight
	installAskUpgrade
	installUpgrading
	installAskReboot
	installRunning
	installDone
)

// installPage 安装流程：确认 → 环境检测 → (升级系统 → 重启) → 安装 → 结果
type installPage struct {
	app     *App
	state   installState
	form    *huh.Form
	confirm bool
	reboot  bool
	tv      *taskView
	pre     task.Preflight
	info    string // 安装完成后的面板信息
	err     error
}

func newInstallPage(app *App) *installPage {
	p := &installPage{app: app, tv: newTaskView(app), confirm: true}
	p.form = yesNo(i18n.T.Get("Install AcePanel to %s?", setupPath), "", i18n.T.Get("Yes"), i18n.T.Get("No"), &p.confirm)
	return p
}

func (p *installPage) init() tea.Cmd { return p.form.Init() }

func (p *installPage) task() *taskView { return p.tv }

func (p *installPage) update(msg tea.Msg) (page, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			if !p.tv.running {
				return backToMenu(p.app)
			}
		case "enter":
			if p.state == installDone {
				return backToMenu(p.app)
			}
		}
	}
	cmd := p.tv.update(msg)

	switch p.state {
	case installConfirm, installAskUpgrade, installAskReboot:
		var fcmd tea.Cmd
		var done bool
		if p.form, fcmd, done = updateForm(p.form, msg); !done {
			return p, tea.Batch(cmd, fcmd)
		}
		return p.formDone()
	case installPreflight, installUpgrading, installRunning:
		if _, ok := msg.(taskDoneMsg); ok {
			return p.taskDone()
		}
	}
	return p, cmd
}

// formDone 表单提交后进入下一阶段
func (p *installPage) formDone() (page, tea.Cmd) {
	switch p.state {
	case installConfirm:
		if !p.confirm {
			return backToMenu(p.app)
		}
		p.state = installPreflight
		return p, p.tv.start(task.PreflightSteps(setupPath, &p.pre))
	case installAskUpgrade:
		if !p.confirm {
			return p.install()
		}
		p.state = installUpgrading
		return p, p.tv.start(task.UpgradeSteps(p.pre.Info))
	case installAskReboot:
		p.app.final = i18n.T.Get("Please run the installer again after the system is back to continue the installation.")
		if !p.reboot {
			return p, tea.Quit
		}
		return p, tea.Sequence(func() tea.Msg {
			_ = sys.Systemctl(context.Background(), "reboot")
			return nil
		}, tea.Quit)
	}
	return p, nil
}

// taskDone 后台任务结束后进入下一阶段
func (p *installPage) taskDone() (page, tea.Cmd) {
	if p.tv.err != nil {
		p.err, p.state = p.tv.err, installDone
		return p, nil
	}
	switch p.state {
	case installPreflight:
		if !p.pre.Outdated {
			return p.install()
		}
		p.state, p.confirm = installAskUpgrade, true
		p.form = yesNo(
			i18n.T.Get("The current system release is outdated. It is strongly recommended to run AcePanel on the latest system release. Upgrade the system now?"),
			"", i18n.T.Get("Upgrade"), i18n.T.Get("Skip"), &p.confirm,
		)
		return p, p.form.Init()
	case installUpgrading:
		if !sys.RebootRequired() {
			return p.install()
		}
		p.state, p.reboot = installAskReboot, true
		p.form = yesNo(
			i18n.T.Get("The system has been upgraded and needs a reboot before continuing"),
			"", i18n.T.Get("Reboot now"), i18n.T.Get("Exit and reboot manually"), &p.reboot,
		)
		return p, p.form.Init()
	case installRunning:
		p.app.final, p.state = p.info, installDone
	}
	return p, nil
}

func (p *installPage) install() (page, tea.Cmd) {
	p.state = installRunning
	return p, p.tv.start(task.InstallSteps(setupPath, p.pre.Info, &p.info))
}

func (p *installPage) view() string {
	var b strings.Builder
	var keys string
	title := i18n.T.Get("Install Panel")
	switch p.state {
	case installConfirm:
		b.WriteString(p.form.View())
		keys = help("Enter", i18n.T.Get("Confirm"), "Esc", i18n.T.Get("Back"))
	case installPreflight:
		title += "  " + p.tv.counter()
		b.WriteString(p.tv.view(p.app.logLines(len(p.tv.steps) + 4)))
		keys = help("Ctrl+C", i18n.T.Get("Abort"))
	case installAskUpgrade, installAskReboot:
		b.WriteString(p.summary() + "\n\n" + p.form.View())
		keys = help("Enter", i18n.T.Get("Confirm"), "Esc", i18n.T.Get("Back"))
	case installUpgrading, installRunning:
		title += "  " + p.tv.counter()
		b.WriteString(p.summary() + "\n\n")
		b.WriteString(p.tv.view(p.app.logLines(lipgloss.Height(b.String()) + len(p.tv.steps) + 4)))
		keys = help("Ctrl+C", i18n.T.Get("Abort"))
	case installDone:
		if p.err != nil {
			title += "  " + p.tv.counter()
			b.WriteString(p.tv.view(0) + "\n" + errorStyle.Render("✗ "+i18n.T.Get("Installation failed")))
		} else {
			b.WriteString(successStyle.Render("✓ " + i18n.T.Get("Installation successful")))
			// 面板信息不加边框也不缩进，方便直接复制；退出后还会再打印一遍
			if p.info != "" {
				b.WriteString("\n\n" + p.info + "\n\n")
				b.WriteString(note(mutedStyle.Render("i"), mutedStyle.Render(i18n.T.Get("The information above will be printed again after exiting the installer."))))
			}
		}
		keys = help("Enter", i18n.T.Get("Back"))
	}
	return layout(title, b.String(), keys)
}

// summary 环境检测得到的系统概况
func (p *installPage) summary() string {
	info := p.pre.Info
	if info == nil {
		return ""
	}
	release := info.Version
	switch {
	case p.pre.Latest != "":
		release += "  " + warnText.Render("→ "+p.pre.Latest)
	case p.pre.Outdated:
		release += "  " + warnText.Render(i18n.T.Get("update available"))
	default:
		release += "  " + mutedStyle.Render(i18n.T.Get("latest"))
	}
	rows := [][2]string{
		{i18n.T.Get("System"), fmt.Sprintf("%s (%s)", info.Name, archName(info.Arch))},
		{i18n.T.Get("Release"), release},
		{i18n.T.Get("Kernel"), info.Kernel},
		{i18n.T.Get("Memory"), fmt.Sprintf("%s, swap %s", humanBytes(info.MemoryMB<<20), humanBytes(info.SwapMB<<20))},
	}
	if info.InChina {
		rows = append(rows, [2]string{i18n.T.Get("Mirror"), "mirrors.tencent.com"})
	}
	return table(rows)
}
