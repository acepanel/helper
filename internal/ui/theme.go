package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

const logo = `   █████╗  ██████╗███████╗██████╗  █████╗ ███╗   ██╗███████╗██╗
  ██╔══██╗██╔════╝██╔════╝██╔══██╗██╔══██╗████╗  ██║██╔════╝██║
  ███████║██║     █████╗  ██████╔╝███████║██╔██╗ ██║█████╗  ██║
  ██╔══██║██║     ██╔══╝  ██╔═══╝ ██╔══██║██║╚██╗██║██╔══╝  ██║
  ██║  ██║╚██████╗███████╗██║     ██║  ██║██║ ╚████║███████╗███████╗
  ╚═╝  ╚═╝ ╚═════╝╚══════╝╚═╝     ╚═╝  ╚═╝╚═╝  ╚═══╝╚══════╝╚══════╝`

var (
	green  = lipgloss.Green
	yellow = lipgloss.Yellow
	red    = lipgloss.Red
	muted  = lipgloss.BrightBlack

	logoStyle    = lipgloss.NewStyle().Bold(true).Foreground(green)
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(green)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(green)
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(yellow)
	errorStyle   = lipgloss.NewStyle().Bold(true).Foreground(red)
	keyStyle     = lipgloss.NewStyle().Foreground(lipgloss.White).Background(muted).Padding(0, 1)
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1)
	warnBox      = boxStyle.BorderForeground(yellow)
	errorBox     = boxStyle.BorderForeground(red)
)

// formTheme huh 表单主题，跟 logo 一样用绿色做强调色
func formTheme(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)
	t.Focused.Base = t.Focused.Base.BorderForeground(green)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Bold(true)
	t.Focused.NoteTitle = t.Focused.Title
	t.Focused.Description = t.Focused.Description.Foreground(muted)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(red)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(red)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(green)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(green)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(lipgloss.Black).Background(green).Bold(true)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(lipgloss.White).Background(muted)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(green)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(green)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(muted)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()
	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}

func newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(formTheme)).WithShowHelp(false)
}

// confirmForm 是/否确认表单，value 的初始值决定默认停在哪个按钮
func confirmForm(title, desc, yes, no string, value *bool) *huh.Form {
	return newForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Description(desc).Affirmative(yes).Negative(no).
			WithButtonAlignment(lipgloss.Left).Value(value),
	))
}

// updateForm 推进表单并返回是否已完成
func updateForm(f *huh.Form, msg tea.Msg) (*huh.Form, tea.Cmd, bool) {
	m, cmd := f.Update(msg)
	if nf, ok := m.(*huh.Form); ok {
		f = nf
	}
	return f, cmd, f.State == huh.StateCompleted
}

// layout 标题、正文、按键提示之间各空一行，标题和提示可为空
func layout(title, body, keys string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(titleStyle.Render(title) + "\n\n")
	}
	b.WriteString(strings.TrimRight(body, "\n"))
	if keys != "" {
		b.WriteString("\n\n" + keys)
	}
	b.WriteString("\n")
	return b.String()
}

// help 底部按键提示，参数成对出现：按键、说明
func help(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, keyStyle.Render(pairs[i])+" "+mutedStyle.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

// table 两列信息表，标签右侧对齐后渲染成一个框
func table(rows [][2]string) string {
	width := 0
	for _, r := range rows {
		width = max(width, lipgloss.Width(r[0]))
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, mutedStyle.Render(r[0]+strings.Repeat(" ", width-lipgloss.Width(r[0])))+"  "+r[1])
	}
	return boxStyle.Render(strings.Join(lines, "\n"))
}

func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v, i := float64(n), 0
	for v >= 1024 && i < len(units)-1 {
		v, i = v/1024, i+1
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0") + " " + units[i]
}

func archName(arch string) string {
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return arch
}

// firstLine 多行错误只取第一行放进列表
func firstLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}
