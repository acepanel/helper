package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/acepanel/helper/pkg/i18n"
)

// setupPath 面板安装目录
const setupPath = "/opt/ace"

// page 一屏交互页面，update 返回下一页（可能还是自己）
type page interface {
	init() tea.Cmd
	update(msg tea.Msg) (page, tea.Cmd)
	view() string
	// task 当前页的任务视图，没有任务时为 nil
	task() *taskView
}

// App 根模型，负责窗口尺寸、全局按键和页面切换
type App struct {
	width, height int
	prog          *tea.Program
	page          page
	quitting      bool   // Ctrl+C 后等任务停下再退出
	final         string // 退出后打印到终端的内容
}

// Run 启动交互界面，返回退出后要打印的内容
func Run() (string, error) {
	app := &App{}
	app.page = newLangPage(app)
	app.prog = tea.NewProgram(app)
	_, err := app.prog.Run()
	return app.final, err
}

func (a *App) Init() tea.Cmd {
	return a.page.init()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if t := a.page.task(); t != nil && t.running {
				t.stop()
				a.quitting = true
				a.final = i18n.T.Get("Aborted")
				return a, nil
			}
			return a, tea.Quit
		}
	}

	// 退出前只等任务收尾，不再推进页面流程
	if a.quitting {
		if t := a.page.task(); t != nil && t.running {
			t.update(msg)
			if t.running {
				return a, nil
			}
		}
		return a, tea.Quit
	}

	var cmd tea.Cmd
	a.page, cmd = a.page.update(msg)
	return a, cmd
}

func (a *App) View() tea.View {
	v := tea.NewView(a.header() + a.page.view())
	v.AltScreen = true
	return v
}

// header 终端够高时显示大 logo，否则只显示一行标题
func (a *App) header() string {
	if a.height < 30 {
		return "\n" + logoStyle.Render(" AcePanel") + "\n\n"
	}
	return logoStyle.Render(logo) + "\n\n"
}

// logLines 命令面板可用的行数，used 为页面其余内容占的行数
func (a *App) logLines(used int) int {
	n := a.height - used - 2
	if a.height >= 30 {
		n -= 8
	} else {
		n -= 3
	}
	if n < 3 {
		return 0
	}
	return min(n, 12)
}
