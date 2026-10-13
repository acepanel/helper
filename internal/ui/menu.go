package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/leonelquinteros/gotext"

	"github.com/acepanel/helper/pkg/embed"
	"github.com/acepanel/helper/pkg/i18n"
)

// langPage 语言选择，选完才初始化翻译
type langPage struct {
	app  *App
	form *huh.Form
	lang string
}

func newLangPage(app *App) *langPage {
	p := &langPage{app: app, lang: "zh_CN"}
	p.form = newForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Select Language / 选择语言").
			Options(
				huh.NewOption("简体中文", "zh_CN"),
				huh.NewOption("繁體中文", "zh_TW"),
				huh.NewOption("English", "en_US"),
			).
			Value(&p.lang),
	))
	return p
}

func (p *langPage) init() tea.Cmd { return p.form.Init() }

func (p *langPage) update(msg tea.Msg) (page, tea.Cmd) {
	var cmd tea.Cmd
	var done bool
	if p.form, cmd, done = updateForm(p.form, msg); !done {
		return p, cmd
	}
	locale := gotext.NewLocaleFSWithPath(p.lang, embed.LocalesFS, "locales")
	locale.AddDomain("helper")
	i18n.T = locale
	next := newMenuPage(p.app)
	return next, next.init()
}

func (p *langPage) view() string { return layout("", p.form.View(), "") }

func (p *langPage) task() *taskView { return nil }

type menuChoice int

const (
	menuInstall menuChoice = iota
	menuUninstall
	menuMount
	menuExit
)

// menuPage 主菜单
type menuPage struct {
	app    *App
	form   *huh.Form
	choice menuChoice
}

func newMenuPage(app *App) *menuPage {
	p := &menuPage{app: app}
	p.form = newForm(huh.NewGroup(
		huh.NewSelect[menuChoice]().
			Title(i18n.T.Get("Select Operation")).
			Options(
				huh.NewOption(i18n.T.Get("Install Panel"), menuInstall),
				huh.NewOption(i18n.T.Get("Uninstall Panel"), menuUninstall),
				huh.NewOption(i18n.T.Get("Disk Partition"), menuMount),
				huh.NewOption(i18n.T.Get("Exit"), menuExit),
			).
			Value(&p.choice),
	))
	return p
}

func (p *menuPage) init() tea.Cmd { return p.form.Init() }

func (p *menuPage) update(msg tea.Msg) (page, tea.Cmd) {
	var cmd tea.Cmd
	var done bool
	if p.form, cmd, done = updateForm(p.form, msg); !done {
		return p, cmd
	}
	var next page
	switch p.choice {
	case menuInstall:
		next = newInstallPage(p.app)
	case menuUninstall:
		next = newUninstallPage(p.app)
	case menuMount:
		next = newMountPage(p.app)
	default:
		return p, tea.Quit
	}
	return next, next.init()
}

func (p *menuPage) view() string {
	return layout("", p.form.View(), help("↑↓", i18n.T.Get("Select"), "Enter", i18n.T.Get("Confirm"), "Ctrl+C", i18n.T.Get("Quit")))
}

func (p *menuPage) task() *taskView { return nil }

// backToMenu 回到主菜单
func backToMenu(app *App) (page, tea.Cmd) {
	next := newMenuPage(app)
	return next, next.init()
}
