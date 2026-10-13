package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/acepanel/helper/internal/task"
	"github.com/acepanel/helper/pkg/i18n"
)

type uninstallState int

const (
	uninstallWarning uninstallState = iota
	uninstallCountdown
	uninstallConfirm
	uninstallRunning
	uninstallDone
)

const (
	countdownSeconds = 60
	countdownSkips   = 10
)

type countdownMsg int

// uninstallPage 卸载流程：警告 → 倒计时 → 确认 → 卸载 → 结果
type uninstallPage struct {
	app       *App
	state     uninstallState
	countdown int
	skips     int // 倒计时期间连按 Enter 的次数，够数就跳过
	form      *huh.Form
	confirm   bool
	tv        *taskView
	err       error
}

func newUninstallPage(app *App) *uninstallPage {
	return &uninstallPage{app: app, tv: newTaskView(app), countdown: countdownSeconds}
}

func (p *uninstallPage) init() tea.Cmd { return nil }

func (p *uninstallPage) task() *taskView { return p.tv }

func (p *uninstallPage) update(msg tea.Msg) (page, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if !p.tv.running {
				return backToMenu(p.app)
			}
		case "enter":
			switch p.state {
			case uninstallWarning:
				p.state = uninstallCountdown
				return p, p.tick()
			case uninstallCountdown:
				if p.skips++; p.skips >= countdownSkips {
					return p, p.askConfirm()
				}
			case uninstallDone:
				return backToMenu(p.app)
			}
		}
	case countdownMsg:
		if p.state != uninstallCountdown {
			return p, nil
		}
		if p.countdown = int(msg); p.countdown <= 0 {
			return p, p.askConfirm()
		}
		return p, p.tick()
	case taskDoneMsg:
		p.tv.update(msg)
		p.err, p.state = p.tv.err, uninstallDone
		return p, nil
	}
	cmd := p.tv.update(msg)

	if p.state == uninstallConfirm {
		var fcmd tea.Cmd
		var done bool
		if p.form, fcmd, done = updateForm(p.form, msg); !done {
			return p, tea.Batch(cmd, fcmd)
		}
		if !p.confirm {
			return backToMenu(p.app)
		}
		p.state = uninstallRunning
		return p, p.tv.start(task.UninstallSteps(setupPath))
	}
	return p, cmd
}

func (p *uninstallPage) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return countdownMsg(p.countdown - 1) })
}

func (p *uninstallPage) askConfirm() tea.Cmd {
	p.state, p.confirm = uninstallConfirm, false
	p.form = yesNo(
		i18n.T.Get("Confirm Uninstallation"),
		i18n.T.Get("Are you sure you want to uninstall the panel?"),
		i18n.T.Get("Yes"), i18n.T.Get("No"), &p.confirm,
	)
	return p.form.Init()
}

func (p *uninstallPage) view() string {
	var b strings.Builder
	var keys string
	title := i18n.T.Get("Uninstall Panel")
	switch p.state {
	case uninstallWarning:
		b.WriteString(note(warnStyle.Render("!"), warnStyle.Render(i18n.T.Get("High-risk operation")),
			i18n.T.Get("Please backup all data before uninstalling."),
			i18n.T.Get("All data will be cleared and cannot be recovered!"),
		))
		keys = help("Enter", i18n.T.Get("Continue"), "Esc", i18n.T.Get("Back"))
	case uninstallCountdown:
		lines := []string{fmt.Sprintf("%s %d %s", i18n.T.Get("Waiting:"), p.countdown, i18n.T.Get("seconds"))}
		if p.skips > 0 {
			lines = append(lines, mutedStyle.Render(i18n.T.Get("Press Enter %d more times to skip", countdownSkips-p.skips)))
		}
		b.WriteString(note(warnStyle.Render("!"), warnStyle.Render(i18n.T.Get("For safety, please wait before proceeding")), lines...))
		keys = help("Enter×10", i18n.T.Get("Skip"), "Esc", i18n.T.Get("Cancel"))
	case uninstallConfirm:
		b.WriteString(p.form.View())
		keys = help("Enter", i18n.T.Get("Confirm"), "Esc", i18n.T.Get("Back"))
	case uninstallRunning:
		title += "  " + p.tv.counter()
		b.WriteString(p.tv.view(p.app.logLines(len(p.tv.steps) + 4)))
		keys = help("Ctrl+C", i18n.T.Get("Abort"))
	case uninstallDone:
		if p.err != nil {
			title += "  " + p.tv.counter()
			b.WriteString(p.tv.view(0) + "\n" + errorStyle.Render("✗ "+i18n.T.Get("Uninstallation failed")))
		} else {
			b.WriteString(successStyle.Render("✓ "+i18n.T.Get("Uninstallation successful")) + "\n\n")
			b.WriteString(i18n.T.Get("Thank you for using AcePanel."))
		}
		keys = help("Enter", i18n.T.Get("Back"))
	}
	return layout(title, b.String(), keys)
}
