package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/acepanel/helper/internal/service"
	"github.com/acepanel/helper/internal/ui"
)

type Helper struct {
	installer   service.Installer
	uninstaller service.Uninstaller
	mounter     service.Mounter
}

func NewHelper(installer service.Installer, uninstaller service.Uninstaller, mounter service.Mounter) *Helper {
	return &Helper{
		installer:   installer,
		uninstaller: uninstaller,
		mounter:     mounter,
	}
}

func (h *Helper) Run() error {
	app := ui.NewApp(h.installer, h.uninstaller, h.mounter)
	p := tea.NewProgram(app)
	app.SetProgram(p)

	_, err := p.Run()
	return err
}
