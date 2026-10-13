package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/internal/task"
	"github.com/acepanel/helper/pkg/i18n"
)

type mountState int

const (
	mountLoading mountState = iota
	mountForm
	mountConfirm
	mountRunning
	mountDone
)

type disksMsg struct {
	disks []sys.Disk
	err   error
}

// mountPage 分区挂载流程：准备工具 → 选盘/挂载点/文件系统 → 确认 → 执行 → 结果
type mountPage struct {
	app     *App
	state   mountState
	spinner spinner.Model
	form    *huh.Form
	confirm bool
	cfg     task.MountConfig
	tv      *taskView
	err     error
}

func newMountPage(app *App) *mountPage {
	p := &mountPage{app: app, tv: newTaskView(app)}
	p.spinner = p.tv.spinner
	p.cfg = task.MountConfig{MountPoint: setupPath, FSType: "ext4"}
	return p
}

func (p *mountPage) init() tea.Cmd {
	return tea.Batch(p.spinner.Tick, func() tea.Msg {
		disks, err := task.PrepareMount(context.Background())
		return disksMsg{disks: disks, err: err}
	})
}

func (p *mountPage) task() *taskView { return p.tv }

func (p *mountPage) update(msg tea.Msg) (page, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if !p.tv.running {
				return backToMenu(p.app)
			}
		case "enter":
			if p.state == mountDone {
				return backToMenu(p.app)
			}
		}
	case spinner.TickMsg:
		if p.state == mountLoading {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			return p, cmd
		}
	case disksMsg:
		if msg.err == nil && len(msg.disks) == 0 {
			msg.err = errors.New(i18n.T.Get("No available disks found"))
		}
		if msg.err != nil {
			p.err, p.state = msg.err, mountDone
			return p, nil
		}
		p.state = mountForm
		p.form = p.newMountForm(msg.disks)
		return p, p.form.Init()
	case taskDoneMsg:
		p.tv.update(msg)
		p.err, p.state = p.tv.err, mountDone
		return p, nil
	}
	cmd := p.tv.update(msg)

	switch p.state {
	case mountForm:
		var fcmd tea.Cmd
		var done bool
		if p.form, fcmd, done = updateForm(p.form, msg); !done {
			return p, tea.Batch(cmd, fcmd)
		}
		p.state, p.confirm = mountConfirm, false
		p.form = confirmForm(
			i18n.T.Get("Confirm Operation"),
			i18n.T.Get("Disk: %s, Mount Point: %s, File System: %s", p.cfg.Disk, p.cfg.MountPoint, p.cfg.FSType),
			i18n.T.Get("Yes"), i18n.T.Get("No"), &p.confirm,
		)
		return p, p.form.Init()
	case mountConfirm:
		var fcmd tea.Cmd
		var done bool
		if p.form, fcmd, done = updateForm(p.form, msg); !done {
			return p, tea.Batch(cmd, fcmd)
		}
		if !p.confirm {
			return backToMenu(p.app)
		}
		p.state = mountRunning
		return p, p.tv.start(task.MountSteps(&p.cfg))
	}
	return p, cmd
}

func (p *mountPage) newMountForm(disks []sys.Disk) *huh.Form {
	options := make([]huh.Option[string], len(disks))
	for i, d := range disks {
		options[i] = huh.NewOption(fmt.Sprintf("%s  %s", d.Path, humanBytes(d.Size)), d.Path)
	}
	return newForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T.Get("Select Disk")).
				Description(i18n.T.Get("Only unused disks are listed, all data on it will be erased")).
				Options(options...).
				Value(&p.cfg.Disk),
		),
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T.Get("Mount Point")).
				Description(i18n.T.Get("Enter the mount point path (e.g. /opt/ace)")).
				Value(&p.cfg.MountPoint).
				Validate(func(s string) error {
					if !strings.HasPrefix(s, "/") {
						return errors.New(i18n.T.Get("Please enter an absolute path"))
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T.Get("File System")).
				Options(
					huh.NewOption("ext4 ("+i18n.T.Get("Recommended")+")", "ext4"),
					huh.NewOption("xfs", "xfs"),
				).
				Value(&p.cfg.FSType),
		),
	)
}

func (p *mountPage) view() string {
	var b strings.Builder
	b.WriteString(title(i18n.T.Get("Disk Partition")))

	switch p.state {
	case mountLoading:
		b.WriteString(" " + p.spinner.View() + " " + i18n.T.Get("Preparing partition tools...") + "\n")
		b.WriteString(help("Esc", i18n.T.Get("Back")))
	case mountForm:
		b.WriteString(p.form.View())
		b.WriteString(help("Enter", i18n.T.Get("Next"), "Shift+Tab", i18n.T.Get("Previous"), "Esc", i18n.T.Get("Back")))
	case mountConfirm:
		b.WriteString(warnBox.Render(warnStyle.Render("⚠ "+i18n.T.Get("Warning: This will erase all data on the disk!"))) + "\n\n")
		b.WriteString(p.form.View())
		b.WriteString(help("Enter", i18n.T.Get("Confirm"), "Esc", i18n.T.Get("Back")))
	case mountRunning:
		b.WriteString(p.tv.view(p.app.logLines(2 + len(p.tv.steps) + 4)))
		b.WriteString(help("Ctrl+C", i18n.T.Get("Abort")))
	case mountDone:
		if p.err != nil {
			b.WriteString(errorStyle.Render("✗ "+i18n.T.Get("Operation failed")) + "\n\n")
			if len(p.tv.steps) > 0 {
				b.WriteString(p.tv.view(0) + "\n")
			}
			b.WriteString(errorBox.Width(min(100, max(40, p.app.width-4))).Render(p.err.Error()) + "\n")
		} else {
			b.WriteString(successStyle.Render("✓ "+i18n.T.Get("Partition and mount successful")) + "\n\n")
			b.WriteString(table([][2]string{
				{i18n.T.Get("Device"), p.cfg.Partition},
				{i18n.T.Get("Mount Point"), p.cfg.MountPoint},
				{i18n.T.Get("File System"), p.cfg.FSType},
			}) + "\n")
		}
		b.WriteString(help("Enter", i18n.T.Get("Back")))
	}
	return b.String()
}
