package ui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/acepanel/helper/pkg/config"
	"github.com/acepanel/helper/pkg/embed"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/leonelquinteros/gotext"

	"github.com/acepanel/helper/internal/service"
	"github.com/acepanel/helper/pkg/i18n"
	"github.com/acepanel/helper/pkg/types"
)

// ViewState 视图状态
type ViewState int

const (
	ViewLanguageSelect ViewState = iota
	ViewMainMenu
	ViewInstall
	ViewUninstall
	ViewMount
)

// MenuChoice 菜单选项
type MenuChoice int

const (
	MenuInstall MenuChoice = iota
	MenuUninstall
	MenuMount
	MenuExit
)

// 消息类型
type (
	progressMsg        types.Progress
	verboseMsg         string // verbose 模式命令输出
	installCompleteMsg struct {
		err  error
		info string
	}
	uninstallCompleteMsg struct{ err error }
	mountCompleteMsg     struct{ err error }
	countdownTickMsg     struct{ remaining int }
	disksLoadedMsg       struct {
		disks []types.DiskInfo
		err   error
	}
)

// App 主应用
type App struct {
	state       ViewState
	width       int
	height      int
	program     *tea.Program
	installer   service.Installer
	uninstaller service.Uninstaller
	mounter     service.Mounter

	// 语言选择
	langForm   *huh.Form
	langChoice string

	// 主菜单
	menuForm   *huh.Form
	menuChoice MenuChoice

	// 安装
	installForm      *huh.Form
	installConfirmed bool
	setupPath        string
	installSpinner   spinner.Model
	installProgress  progress.Model
	installStep      string
	installPercent   float64
	installLogs      []string
	installErr       error
	installRunning   bool
	installDone      bool
	installInfo      string // 安装完成后的面板信息

	// verbose 模式
	verboseLogs     []string
	verboseViewport viewport.Model

	// 卸载
	uninstallCountdown int
	uninstallSkipCount int // 连按enter跳过倒计时的计数
	uninstallForm      *huh.Form
	uninstallConfirm   bool
	uninstallSpinner   spinner.Model
	uninstallStep      string
	uninstallLogs      []string
	uninstallErr       error
	uninstallRunning   bool
	uninstallDone      bool
	uninstallState     int // 0=warning, 1=countdown, 2=confirm, 3=running, 4=done

	// 磁盘分区
	disks            []types.DiskInfo
	mountDiskForm    *huh.Form
	mountPointForm   *huh.Form
	mountFSForm      *huh.Form
	mountConfirmForm *huh.Form
	mountConfirmed   bool
	mountConfig      types.MountConfig
	mountSpinner     spinner.Model
	mountStep        string
	mountLogs        []string
	mountErr         error
	mountRunning     bool
	mountDone        bool
	mountState       int // 0=loading, 1=selectDisk, 2=selectMount, 3=selectFS, 4=confirm, 5=running, 6=done
}

// NewApp 创建应用
func NewApp(installer service.Installer, uninstaller service.Uninstaller, mounter service.Mounter) *App {
	app := &App{
		state:       ViewLanguageSelect,
		langChoice:  "zh_CN",
		installer:   installer,
		uninstaller: uninstaller,
		mounter:     mounter,
		setupPath:   "/opt/ace",
		mountConfig: types.MountConfig{
			MountPoint: "/opt/ace",
			FSType:     types.FSTypeExt4,
		},
	}

	// 初始化 spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorPrimary)
	app.installSpinner = s
	app.uninstallSpinner = s
	app.mountSpinner = s

	// 初始化进度条
	app.installProgress = progress.New(progress.WithDefaultGradient())

	// 初始化 verbose viewport
	app.verboseViewport = viewport.New(80, 8)
	app.verboseViewport.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorMuted).
		Padding(0, 1)

	return app
}

func (a *App) SetProgram(p *tea.Program) {
	a.program = p
}

func (a *App) Init() tea.Cmd {
	a.langForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select Language / 选择语言").
				Options(
					huh.NewOption("简体中文", "zh_CN"),
					huh.NewOption("繁體中文", "zh_TW"),
					huh.NewOption("English", "en_US"),
				).
				Value(&a.langChoice),
		),
	).WithTheme(huh.ThemeDracula())

	return a.langForm.Init()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.installProgress.Width = msg.Width - 10

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	switch a.state {
	case ViewLanguageSelect:
		return a.updateLanguageSelect(msg)
	case ViewMainMenu:
		return a.updateMainMenu(msg)
	case ViewInstall:
		return a.updateInstall(msg)
	case ViewUninstall:
		return a.updateUninstall(msg)
	case ViewMount:
		return a.updateMount(msg)
	}

	return a, nil
}

func (a *App) View() string {
	switch a.state {
	case ViewLanguageSelect:
		return a.viewLanguageSelect()
	case ViewMainMenu:
		return a.viewMainMenu()
	case ViewInstall:
		return a.viewInstall()
	case ViewUninstall:
		return a.viewUninstall()
	case ViewMount:
		return a.viewMount()
	}
	return ""
}

// ========== 语言选择 ==========

func (a *App) updateLanguageSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := a.langForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.langForm = f
	}

	if a.langForm.State == huh.StateCompleted {
		locale := gotext.NewLocaleFSWithPath(a.langChoice, embed.LocalesFS, "locales")
		locale.AddDomain("helper")
		i18n.T = locale
		a.state = ViewMainMenu
		return a, a.initMainMenu()
	}

	return a, cmd
}

func (a *App) viewLanguageSelect() string {
	return RenderLogo() + "\n" + a.langForm.View()
}

// ========== 主菜单 ==========

func (a *App) initMainMenu() tea.Cmd {
	a.menuForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[MenuChoice]().
				Title(i18n.T.Get("Select Operation")).
				Options(
					huh.NewOption(i18n.T.Get("Install Panel"), MenuInstall),
					huh.NewOption(i18n.T.Get("Uninstall Panel"), MenuUninstall),
					huh.NewOption(i18n.T.Get("Disk Partition"), MenuMount),
					huh.NewOption(i18n.T.Get("Exit"), MenuExit),
				).
				Value(&a.menuChoice),
		),
	).WithTheme(huh.ThemeDracula())

	return a.menuForm.Init()
}

func (a *App) updateMainMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := a.menuForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.menuForm = f
	}

	if a.menuForm.State == huh.StateCompleted {
		switch a.menuChoice {
		case MenuInstall:
			a.state = ViewInstall
			a.installRunning = false
			a.installDone = false
			a.installErr = nil
			a.installLogs = nil
			return a, a.initInstall()
		case MenuUninstall:
			a.state = ViewUninstall
			a.uninstallState = 0
			a.uninstallRunning = false
			a.uninstallDone = false
			a.uninstallErr = nil
			a.uninstallLogs = nil
			a.uninstallCountdown = 60
			return a, nil
		case MenuMount:
			a.state = ViewMount
			a.mountState = 0
			a.mountRunning = false
			a.mountDone = false
			a.mountErr = nil
			a.mountLogs = nil
			return a, tea.Batch(a.mountSpinner.Tick, a.loadDisks())
		case MenuExit:
			return a, tea.Quit
		}
	}

	return a, cmd
}

func (a *App) viewMainMenu() string {
	return RenderLogo() + "\n" + a.menuForm.View()
}

// ========== 安装面板 ==========

func (a *App) initInstall() tea.Cmd {
	a.installForm = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.T.Get("Confirm Installation")).
				Description(i18n.T.Get("Panel will be installed to %s", a.setupPath)).
				Affirmative(i18n.T.Get("Yes")).
				Negative(i18n.T.Get("No")).
				Value(&a.installConfirmed),
		),
	).WithTheme(huh.ThemeDracula())

	return a.installForm.Init()
}

func (a *App) updateInstall(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" && !a.installRunning {
			a.state = ViewMainMenu
			return a, a.initMainMenu()
		}
		if msg.String() == "enter" && a.installDone {
			a.state = ViewMainMenu
			return a, a.initMainMenu()
		}

	case progressMsg:
		a.installStep = msg.Step
		a.installPercent = msg.Percent
		if msg.Message != "" {
			a.installLogs = append(a.installLogs, msg.Message)
			if len(a.installLogs) > 8 {
				a.installLogs = a.installLogs[1:]
			}
		}
		return a, nil

	case verboseMsg:
		a.verboseLogs = append(a.verboseLogs, string(msg))
		// 保留最近 100 条
		if len(a.verboseLogs) > 100 {
			a.verboseLogs = a.verboseLogs[len(a.verboseLogs)-100:]
		}
		a.verboseViewport.SetContent(strings.Join(a.verboseLogs, "\n"))
		a.verboseViewport.GotoBottom()
		return a, nil

	case installCompleteMsg:
		a.installRunning = false
		a.installDone = true
		a.installErr = msg.err
		a.installInfo = msg.info
		return a, nil

	case spinner.TickMsg:
		if a.installRunning {
			var cmd tea.Cmd
			a.installSpinner, cmd = a.installSpinner.Update(msg)
			return a, cmd
		}
	}

	if !a.installRunning && !a.installDone {
		form, cmd := a.installForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			a.installForm = f
		}

		if a.installForm.State == huh.StateCompleted {
			if a.installConfirmed {
				a.installRunning = true
				a.verboseLogs = nil // 清空 verbose 日志
				return a, tea.Batch(a.installSpinner.Tick, a.startInstall())
			}
			a.state = ViewMainMenu
			return a, a.initMainMenu()
		}
		return a, cmd
	}

	return a, nil
}

func (a *App) startInstall() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		progressCh := make(chan types.Progress, 10)

		go func() {
			for p := range progressCh {
				if a.program != nil {
					a.program.Send(progressMsg(p))
				}
			}
		}()

		// 设置 verbose 回调
		if config.Global.Verbose {
			a.installer.SetVerboseCallback(func(cmd, stdout, stderr string, err error) {
				if a.program != nil {
					msg := "$ " + cmd
					if stdout != "" {
						msg += "\n" + strings.TrimSpace(stdout)
					}
					if stderr != "" {
						msg += "\n[stderr] " + strings.TrimSpace(stderr)
					}
					if err != nil {
						msg += "\n[error] " + err.Error()
					}
					a.program.Send(verboseMsg(msg))
				}
			})
		}

		cfg := &types.InstallConfig{
			SetupPath: a.setupPath,
			AutoSwap:  true,
		}

		err := a.installer.Install(ctx, cfg, progressCh)
		close(progressCh)
		time.Sleep(100 * time.Millisecond)

		// 安装成功后获取面板信息
		var info string
		if err == nil {
			cmd := exec.CommandContext(ctx, "/usr/local/sbin/acepanel", "info")
			output, _ := cmd.Output()
			info = string(output)
		}

		return installCompleteMsg{err: err, info: info}
	}
}

func (a *App) viewInstall() string {
	var sb strings.Builder
	sb.WriteString(RenderLogo())
	sb.WriteString("\n")
	sb.WriteString(RenderTitle(i18n.T.Get("Install Panel")))
	sb.WriteString("\n")

	if a.installDone {
		if a.installErr != nil {
			sb.WriteString(RenderError(i18n.T.Get("Installation failed")))
			sb.WriteString("\n\n")
			sb.WriteString(ErrorBoxStyle.Render(a.installErr.Error()))
		} else {
			sb.WriteString(RenderSuccess(i18n.T.Get("Installation successful")))
			if a.installInfo != "" {
				sb.WriteString("\n\n")
				sb.WriteString(InfoBoxStyle.Render(strings.TrimSpace(a.installInfo)))
			}
		}
		sb.WriteString("\n\n")
		sb.WriteString(RenderHelp("Enter", i18n.T.Get("Back")))
	} else if a.installRunning {
		sb.WriteString(fmt.Sprintf("%s %s\n\n", a.installSpinner.View(), a.installStep))
		sb.WriteString(a.installProgress.ViewAs(a.installPercent))
		sb.WriteString("\n\n")
		for _, log := range a.installLogs {
			sb.WriteString(LogStyle.Render(log))
			sb.WriteString("\n")
		}
		// verbose 模式显示命令日志
		if config.Global.Verbose && len(a.verboseLogs) > 0 {
			sb.WriteString("\n")
			sb.WriteString(MutedStyle.Render("Commands:"))
			sb.WriteString("\n")
			sb.WriteString(a.verboseViewport.View())
		}
	} else {
		sb.WriteString(a.installForm.View())
	}

	return sb.String()
}

// ========== 卸载面板 ==========

func (a *App) updateUninstall(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if !a.uninstallRunning {
				a.state = ViewMainMenu
				return a, a.initMainMenu()
			}
		case "enter":
			if a.uninstallState == 0 {
				a.uninstallState = 1
				a.uninstallSkipCount = 0
				return a, a.tickCountdown()
			}
			if a.uninstallState == 1 {
				// 倒计时期间连按10次enter可跳过
				a.uninstallSkipCount++
				if a.uninstallSkipCount >= 10 {
					a.uninstallState = 2
					a.initUninstallConfirm()
					return a, a.uninstallForm.Init()
				}
			}
			if a.uninstallDone {
				a.state = ViewMainMenu
				return a, a.initMainMenu()
			}
		}

	case countdownTickMsg:
		a.uninstallCountdown = msg.remaining
		if a.uninstallCountdown <= 0 {
			a.uninstallState = 2
			a.initUninstallConfirm()
			return a, a.uninstallForm.Init()
		}
		return a, a.tickCountdown()

	case progressMsg:
		a.uninstallStep = msg.Step
		if msg.Message != "" {
			a.uninstallLogs = append(a.uninstallLogs, msg.Message)
			if len(a.uninstallLogs) > 8 {
				a.uninstallLogs = a.uninstallLogs[1:]
			}
		}
		return a, nil

	case verboseMsg:
		a.verboseLogs = append(a.verboseLogs, string(msg))
		if len(a.verboseLogs) > 100 {
			a.verboseLogs = a.verboseLogs[len(a.verboseLogs)-100:]
		}
		a.verboseViewport.SetContent(strings.Join(a.verboseLogs, "\n"))
		a.verboseViewport.GotoBottom()
		return a, nil

	case uninstallCompleteMsg:
		a.uninstallRunning = false
		a.uninstallDone = true
		a.uninstallState = 4
		a.uninstallErr = msg.err
		return a, nil

	case spinner.TickMsg:
		if a.uninstallRunning {
			var cmd tea.Cmd
			a.uninstallSpinner, cmd = a.uninstallSpinner.Update(msg)
			return a, cmd
		}
	}

	if a.uninstallState == 2 {
		form, cmd := a.uninstallForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			a.uninstallForm = f
		}

		if a.uninstallForm.State == huh.StateCompleted {
			if a.uninstallConfirm {
				a.uninstallState = 3
				a.uninstallRunning = true
				a.verboseLogs = nil // 清空 verbose 日志
				return a, tea.Batch(a.uninstallSpinner.Tick, a.startUninstall())
			}
			a.state = ViewMainMenu
			return a, a.initMainMenu()
		}
		return a, cmd
	}

	return a, nil
}

func (a *App) initUninstallConfirm() {
	a.uninstallConfirm = false
	a.uninstallForm = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.T.Get("Confirm Uninstallation")).
				Description(i18n.T.Get("Are you sure you want to uninstall the panel?")).
				Affirmative(i18n.T.Get("Yes")).
				Negative(i18n.T.Get("No")).
				Value(&a.uninstallConfirm),
		),
	).WithTheme(huh.ThemeDracula())
}

func (a *App) tickCountdown() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return countdownTickMsg{remaining: a.uninstallCountdown - 1}
	})
}

func (a *App) startUninstall() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// 设置 verbose 回调
		if config.Global.Verbose {
			a.uninstaller.SetVerboseCallback(func(cmd, stdout, stderr string, err error) {
				if a.program != nil {
					msg := "$ " + cmd
					if stdout != "" {
						msg += "\n" + strings.TrimSpace(stdout)
					}
					if stderr != "" {
						msg += "\n[stderr] " + strings.TrimSpace(stderr)
					}
					if err != nil {
						msg += "\n[error] " + err.Error()
					}
					a.program.Send(verboseMsg(msg))
				}
			})
		}

		err := a.uninstaller.Uninstall(ctx, a.setupPath, func(step, message string) {
			if a.program != nil {
				a.program.Send(progressMsg{Step: step, Message: message})
			}
		})
		return uninstallCompleteMsg{err: err}
	}
}

func (a *App) viewUninstall() string {
	var sb strings.Builder
	sb.WriteString(RenderLogo())
	sb.WriteString("\n")
	sb.WriteString(RenderTitle(i18n.T.Get("Uninstall Panel")))
	sb.WriteString("\n")

	switch a.uninstallState {
	case 0: // warning
		sb.WriteString(WarningBoxStyle.Render(
			RenderWarning(i18n.T.Get("High-risk operation")) + "\n\n" +
				i18n.T.Get("Please backup all data before uninstalling.") + "\n" +
				i18n.T.Get("All data will be cleared and cannot be recovered!"),
		))
		sb.WriteString("\n\n")
		sb.WriteString(RenderHelp("Enter", i18n.T.Get("Continue"), "Esc", i18n.T.Get("Back")))

	case 1: // countdown
		sb.WriteString(WarningBoxStyle.Render(
			fmt.Sprintf("%s\n\n%s %d %s",
				i18n.T.Get("For safety, please wait before proceeding"),
				i18n.T.Get("Waiting:"),
				a.uninstallCountdown,
				i18n.T.Get("seconds"),
			),
		))
		sb.WriteString("\n\n")
		if a.uninstallSkipCount > 0 {
			sb.WriteString(MutedStyle.Render(fmt.Sprintf("Press Enter %d more times to skip", 10-a.uninstallSkipCount)))
			sb.WriteString("\n")
		}
		sb.WriteString(RenderHelp("Esc", i18n.T.Get("Cancel"), "Enter×10", i18n.T.Get("Skip")))

	case 2: // confirm
		sb.WriteString(a.uninstallForm.View())

	case 3: // running
		sb.WriteString(fmt.Sprintf("%s %s\n\n", a.uninstallSpinner.View(), a.uninstallStep))
		for _, log := range a.uninstallLogs {
			sb.WriteString(LogStyle.Render(log))
			sb.WriteString("\n")
		}
		// verbose 模式显示命令日志
		if config.Global.Verbose && len(a.verboseLogs) > 0 {
			sb.WriteString("\n")
			sb.WriteString(MutedStyle.Render("Commands:"))
			sb.WriteString("\n")
			sb.WriteString(a.verboseViewport.View())
		}

	case 4: // done
		if a.uninstallErr != nil {
			sb.WriteString(RenderError(i18n.T.Get("Uninstallation failed")))
			sb.WriteString("\n\n")
			sb.WriteString(ErrorBoxStyle.Render(a.uninstallErr.Error()))
		} else {
			sb.WriteString(RenderSuccess(i18n.T.Get("Uninstallation successful")))
			sb.WriteString("\n\n")
			sb.WriteString(i18n.T.Get("Thank you for using AcePanel."))
		}
		sb.WriteString("\n\n")
		sb.WriteString(RenderHelp("Enter", i18n.T.Get("Back")))
	}

	return sb.String()
}

// ========== 磁盘分区 ==========

func (a *App) loadDisks() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		disks, err := a.mounter.ListDisks(ctx)
		return disksLoadedMsg{disks: disks, err: err}
	}
}

func (a *App) updateMount(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if !a.mountRunning {
				a.state = ViewMainMenu
				return a, a.initMainMenu()
			}
		case "enter":
			if a.mountDone {
				a.state = ViewMainMenu
				return a, a.initMainMenu()
			}
		}

	case disksLoadedMsg:
		if msg.err != nil {
			a.mountDone = true
			a.mountErr = msg.err
			return a, nil
		}
		if len(msg.disks) == 0 {
			a.mountDone = true
			a.mountErr = errors.New(i18n.T.Get("No available disks found"))
			return a, nil
		}
		a.disks = msg.disks
		a.mountState = 1
		a.initDiskForm()
		return a, a.mountDiskForm.Init()

	case progressMsg:
		a.mountStep = msg.Step
		if msg.Message != "" {
			a.mountLogs = append(a.mountLogs, msg.Message)
			if len(a.mountLogs) > 8 {
				a.mountLogs = a.mountLogs[1:]
			}
		}
		return a, nil

	case verboseMsg:
		a.verboseLogs = append(a.verboseLogs, string(msg))
		if len(a.verboseLogs) > 100 {
			a.verboseLogs = a.verboseLogs[len(a.verboseLogs)-100:]
		}
		a.verboseViewport.SetContent(strings.Join(a.verboseLogs, "\n"))
		a.verboseViewport.GotoBottom()
		return a, nil

	case mountCompleteMsg:
		a.mountRunning = false
		a.mountDone = true
		a.mountErr = msg.err
		return a, nil

	case spinner.TickMsg:
		if a.mountState == 0 || a.mountRunning {
			var cmd tea.Cmd
			a.mountSpinner, cmd = a.mountSpinner.Update(msg)
			return a, cmd
		}
	}

	switch a.mountState {
	case 1: // select disk
		form, cmd := a.mountDiskForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			a.mountDiskForm = f
		}
		if a.mountDiskForm.State == huh.StateCompleted {
			a.mountState = 2
			a.initMountPointForm()
			return a, a.mountPointForm.Init()
		}
		return a, cmd

	case 2: // select mount point
		form, cmd := a.mountPointForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			a.mountPointForm = f
		}
		if a.mountPointForm.State == huh.StateCompleted {
			a.mountState = 3
			a.initFSForm()
			return a, a.mountFSForm.Init()
		}
		return a, cmd

	case 3: // select fs
		form, cmd := a.mountFSForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			a.mountFSForm = f
		}
		if a.mountFSForm.State == huh.StateCompleted {
			a.mountState = 4
			a.initMountConfirmForm()
			return a, a.mountConfirmForm.Init()
		}
		return a, cmd

	case 4: // confirm
		form, cmd := a.mountConfirmForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			a.mountConfirmForm = f
		}
		if a.mountConfirmForm.State == huh.StateCompleted {
			if a.mountConfirmed {
				a.mountState = 5
				a.mountRunning = true
				a.verboseLogs = nil // 清空 verbose 日志
				return a, tea.Batch(a.mountSpinner.Tick, a.startMount())
			}
			a.state = ViewMainMenu
			return a, a.initMainMenu()
		}
		return a, cmd
	}

	return a, nil
}

func (a *App) initDiskForm() {
	options := make([]huh.Option[string], len(a.disks))
	for i, disk := range a.disks {
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", disk.Name, disk.Size), disk.Name)
	}

	a.mountDiskForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T.Get("Select Disk")).
				Description(i18n.T.Get("Select a disk to partition and mount")).
				Options(options...).
				Value(&a.mountConfig.Disk),
		),
	).WithTheme(huh.ThemeDracula())
}

func (a *App) initMountPointForm() {
	a.mountPointForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T.Get("Mount Point")).
				Description(i18n.T.Get("Enter the mount point path (e.g. /opt/ace)")).
				Value(&a.mountConfig.MountPoint).
				Validate(func(s string) error {
					if len(s) == 0 || s[0] != '/' {
						return errors.New(i18n.T.Get("Please enter an absolute path"))
					}
					return nil
				}),
		),
	).WithTheme(huh.ThemeDracula())
}

func (a *App) initFSForm() {
	a.mountFSForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[types.FSType]().
				Title(i18n.T.Get("File System")).
				Description(i18n.T.Get("Select the file system type")).
				Options(
					huh.NewOption(fmt.Sprintf("ext4 (%s)", i18n.T.Get("Recommended")), types.FSTypeExt4),
					huh.NewOption("xfs", types.FSTypeXFS),
				).
				Value(&a.mountConfig.FSType),
		),
	).WithTheme(huh.ThemeDracula())
}

func (a *App) initMountConfirmForm() {
	a.mountConfirmed = false
	a.mountConfirmForm = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.T.Get("Confirm Operation")).
				Description(i18n.T.Get("Disk: %s, Mount Point: %s, File System: %s",
					a.mountConfig.Disk,
					a.mountConfig.MountPoint,
					a.mountConfig.FSType,
				)).
				Affirmative(i18n.T.Get("Yes")).
				Negative(i18n.T.Get("No")).
				Value(&a.mountConfirmed),
		),
	).WithTheme(huh.ThemeDracula())
}

func (a *App) startMount() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// 设置 verbose 回调
		if config.Global.Verbose {
			a.mounter.SetVerboseCallback(func(cmd, stdout, stderr string, err error) {
				if a.program != nil {
					msg := "$ " + cmd
					if stdout != "" {
						msg += "\n" + strings.TrimSpace(stdout)
					}
					if stderr != "" {
						msg += "\n[stderr] " + strings.TrimSpace(stderr)
					}
					if err != nil {
						msg += "\n[error] " + err.Error()
					}
					a.program.Send(verboseMsg(msg))
				}
			})
		}

		err := a.mounter.Mount(ctx, &a.mountConfig, func(step, message string) {
			if a.program != nil {
				a.program.Send(progressMsg{Step: step, Message: message})
			}
		})
		return mountCompleteMsg{err: err}
	}
}

func (a *App) viewMount() string {
	var sb strings.Builder
	sb.WriteString(RenderLogo())
	sb.WriteString("\n")
	sb.WriteString(RenderTitle(i18n.T.Get("Disk Partition")))
	sb.WriteString("\n")

	if a.mountDone {
		if a.mountErr != nil {
			sb.WriteString(RenderError(i18n.T.Get("Operation failed")))
			sb.WriteString("\n\n")
			sb.WriteString(ErrorBoxStyle.Render(a.mountErr.Error()))
		} else {
			sb.WriteString(RenderSuccess(i18n.T.Get("Partition and mount successful")))
			sb.WriteString("\n\n")
			sb.WriteString(BoxStyle.Render(fmt.Sprintf(
				"%s: /dev/%s1\n%s: %s\n%s: %s",
				i18n.T.Get("Device"), a.mountConfig.Disk,
				i18n.T.Get("Mount Point"), a.mountConfig.MountPoint,
				i18n.T.Get("File System"), a.mountConfig.FSType,
			)))
		}
		sb.WriteString("\n\n")
		sb.WriteString(RenderHelp("Enter", i18n.T.Get("Back")))
		return sb.String()
	}

	switch a.mountState {
	case 0: // loading
		sb.WriteString(fmt.Sprintf("%s %s", a.mountSpinner.View(), i18n.T.Get("Loading disk list...")))

	case 1: // select disk
		sb.WriteString(i18n.T.Get("Available disks:"))
		sb.WriteString("\n\n")
		for _, disk := range a.disks {
			sb.WriteString(fmt.Sprintf("  • %s (%s)\n", disk.Name, disk.Size))
		}
		sb.WriteString("\n")
		sb.WriteString(a.mountDiskForm.View())

	case 2: // select mount point
		sb.WriteString(a.mountPointForm.View())

	case 3: // select fs
		sb.WriteString(a.mountFSForm.View())

	case 4: // confirm
		sb.WriteString(WarningBoxStyle.Render(i18n.T.Get("Warning: This will erase all data on the disk!")))
		sb.WriteString("\n\n")
		sb.WriteString(a.mountConfirmForm.View())

	case 5: // running
		sb.WriteString(fmt.Sprintf("%s %s\n\n", a.mountSpinner.View(), a.mountStep))
		for _, log := range a.mountLogs {
			sb.WriteString(LogStyle.Render(log))
			sb.WriteString("\n")
		}
		// verbose 模式显示命令日志
		if config.Global.Verbose && len(a.verboseLogs) > 0 {
			sb.WriteString("\n")
			sb.WriteString(MutedStyle.Render("Commands:"))
			sb.WriteString("\n")
			sb.WriteString(a.verboseViewport.View())
		}
	}

	return sb.String()
}
