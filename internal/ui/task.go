package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
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

// taskView 步骤列表就是进度本身，安装/卸载/分区共用
type taskView struct {
	app       *App
	steps     []task.Step
	state     []stepState
	errs      []error
	spinner   spinner.Model
	lines     []string // verbose 模式下的命令输出
	lastCmd   string
	cancel    context.CancelFunc
	running   bool
	cancelled bool
	err       error
}

func newTaskView(app *App) *taskView {
	return &taskView{
		app:     app,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(lipgloss.NewStyle().Foreground(green))),
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
		if msg.cmd != "" {
			t.lastCmd = msg.cmd
			t.lines = append(t.lines, msg.cmd)
		}
		if msg.output != "" {
			t.lines = append(t.lines, strings.Split(msg.output, "\n")...)
		}
		if len(t.lines) > 200 {
			t.lines = t.lines[len(t.lines)-200:]
		}
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

// counter 已完成/总数，放在页面标题旁边
func (t *taskView) counter() string {
	done := 0
	for _, s := range t.state {
		if s == stepDone || s == stepWarn {
			done++
		}
	}
	return mutedStyle.Render(fmt.Sprintf("%d/%d", done, len(t.steps)))
}

// view 渲染步骤列表，maxLog 是 verbose 模式下当前步骤下方可显示的输出行数
func (t *taskView) view(maxLog int) string {
	var b strings.Builder
	w := t.app.contentWidth()
	for i, s := range t.steps {
		switch t.state[i] {
		case stepRunning:
			_, _ = fmt.Fprintf(&b, "%s %s\n", t.spinner.View(), s.Name)
			b.WriteString(t.output(maxLog, w))
		case stepDone:
			_, _ = fmt.Fprintf(&b, "%s %s\n", successStyle.Render("✓"), s.Name)
		case stepWarn:
			_, _ = fmt.Fprintf(&b, "%s %s\n", warnStyle.Render("!"), s.Name)
			b.WriteString(reason(warnText, firstLine(t.errs[i]), w) + "\n")
		case stepFailed:
			_, _ = fmt.Fprintf(&b, "%s %s\n", errorStyle.Render("✗"), s.Name)
			b.WriteString(reason(errorText, t.errs[i].Error(), w) + "\n")
		default:
			_, _ = fmt.Fprintf(&b, "%s %s\n", mutedStyle.Render("○"), mutedStyle.Render(s.Name))
		}
	}
	if t.running && t.cancelled {
		b.WriteString("\n" + note(warnStyle.Render("!"), warnText.Render(i18n.T.Get("Aborting, waiting for the current command to stop..."))) + "\n")
	}
	return b.String()
}

// output 当前步骤下方的命令回显：verbose 模式给最近几行输出，否则只给正在执行的命令
func (t *taskView) output(maxLog, width int) string {
	lines := []string{t.lastCmd}
	if sys.Verbose && maxLog > 0 {
		lines = t.lines
		if len(lines) > maxLog {
			lines = lines[len(lines)-maxLog:]
		}
	}
	var b strings.Builder
	for _, l := range lines {
		if l != "" {
			b.WriteString("  " + mutedStyle.MaxWidth(width-2).Inline(true).Render(l) + "\n")
		}
	}
	return b.String()
}
