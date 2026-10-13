package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/acepanel/helper/internal/sys"
	"github.com/acepanel/helper/internal/task"
	"github.com/acepanel/helper/pkg/i18n"
)

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepWarn
	stepFailed
)

type (
	stepMsg     task.Event
	cmdMsg      struct{ cmd, output string }
	taskDoneMsg struct{ err error }
)

// taskView 步骤列表、进度条和命令输出，安装/卸载/分区共用
type taskView struct {
	app       *App
	steps     []task.Step
	state     []stepState
	errs      []error
	spinner   spinner.Model
	bar       progress.Model
	logs      viewport.Model
	lines     []string
	lastCmd   string
	cancel    context.CancelFunc
	running   bool
	cancelled bool
	err       error
}

func newTaskView(app *App) *taskView {
	return &taskView{
		app:     app,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(green))),
		bar:     progress.New(progress.WithDefaultBlend(), progress.WithWidth(40), progress.WithFillCharacters('█', '░')),
		logs:    viewport.New(viewport.WithWidth(76), viewport.WithHeight(8)),
	}
}

// start 在后台顺序执行步骤，进度通过 Program.Send 送回
func (t *taskView) start(steps []task.Step) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	t.steps, t.cancel = steps, cancel
	t.state = make([]stepState, len(steps))
	t.errs = make([]error, len(steps))
	t.lines, t.lastCmd = nil, ""
	t.running, t.cancelled, t.err = true, false, nil
	t.logs.SetContent("")
	sys.OnCommand = func(cmd, output string) { t.app.prog.Send(cmdMsg{cmd: cmd, output: output}) }
	return tea.Batch(t.spinner.Tick, func() tea.Msg {
		return taskDoneMsg{err: task.Run(ctx, steps, func(e task.Event) { t.app.prog.Send(stepMsg(e)) })}
	})
}

func (t *taskView) stop() {
	if t.cancel != nil {
		t.cancel()
		t.cancelled = true
	}
}

func (t *taskView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.bar.SetWidth(min(60, max(20, msg.Width-8)))
		t.logs.SetWidth(max(20, msg.Width-6))
	case stepMsg:
		switch {
		case msg.Err != nil && !msg.Done:
			t.state[msg.Step], t.errs[msg.Step] = stepFailed, msg.Err
		case msg.Err != nil:
			t.state[msg.Step], t.errs[msg.Step] = stepWarn, msg.Err
		case msg.Done:
			t.state[msg.Step] = stepDone
		default:
			t.state[msg.Step] = stepRunning
		}
	case cmdMsg:
		t.lastCmd = msg.cmd
		t.lines = append(t.lines, msg.cmd)
		if msg.output != "" {
			t.lines = append(t.lines, strings.Split(msg.output, "\n")...)
		}
		if len(t.lines) > 200 {
			t.lines = t.lines[len(t.lines)-200:]
		}
		t.logs.SetContent(strings.Join(t.lines, "\n"))
		t.logs.GotoBottom()
	case taskDoneMsg:
		t.running, t.err = false, msg.err
		t.cancel()
		// 任务结束后 Update 里不能再有命令回调，否则 Send 会和事件循环互相等待
		sys.OnCommand = nil
	case spinner.TickMsg:
		if t.running {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(msg)
			return cmd
		}
	}
	return nil
}

// view 渲染步骤列表，maxLog 是命令面板可用行数，0 表示只显示最近一条命令
func (t *taskView) view(maxLog int) string {
	var b strings.Builder
	done := 0
	for i, s := range t.steps {
		var icon string
		name := s.Name
		switch t.state[i] {
		case stepRunning:
			icon = t.spinner.View()
		case stepDone:
			icon, done = successStyle.Render("✓"), done+1
		case stepWarn:
			icon, done = warnStyle.Render("!"), done+1
		case stepFailed:
			icon = errorStyle.Render("✗")
		default:
			icon, name = mutedStyle.Render("○"), mutedStyle.Render(name)
		}
		_, _ = fmt.Fprintf(&b, " %s %s\n", icon, name)
		if t.state[i] == stepWarn {
			_, _ = fmt.Fprintf(&b, "   %s\n", warnStyle.Render(firstLine(t.errs[i])))
		}
	}
	if len(t.steps) > 1 {
		b.WriteString("\n " + t.bar.ViewAs(float64(done)/float64(len(t.steps))) + "\n")
	}
	if !t.running {
		return b.String()
	}
	if t.cancelled {
		b.WriteString("\n " + warnStyle.Render(i18n.T.Get("Aborting, waiting for the current command to stop...")) + "\n")
	}
	if sys.Verbose && maxLog > 0 {
		t.logs.SetHeight(maxLog)
		b.WriteString("\n" + boxStyle.Render(t.logs.View()) + "\n")
	} else if t.lastCmd != "" {
		b.WriteString("\n " + mutedStyle.MaxWidth(max(20, t.app.width-2)).Inline(true).Render(t.lastCmd) + "\n")
	}
	return b.String()
}
