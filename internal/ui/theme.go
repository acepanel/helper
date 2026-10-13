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

// 只用四种颜色：绿是标题、完成和光标，黄是警告，红是错误，灰是说明和未开始
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
	warnText     = lipgloss.NewStyle().Foreground(yellow)
	errorText    = lipgloss.NewStyle().Foreground(red)
	keyStyle     = lipgloss.NewStyle().Bold(true)
)

// formTheme 表单跟其它块共用同一套语法：问题以 ? 开头，光标是 ›，说明缩进两列，没有边框
func formTheme(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)
	t.Focused.Base = lipgloss.NewStyle()
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = titleStyle.SetString("?")
	t.Focused.NoteTitle = t.Focused.Title
	t.Focused.Description = mutedStyle.SetString(" ")
	t.Focused.ErrorIndicator = errorText.SetString(" ✗")
	t.Focused.ErrorMessage = errorText.SetString("  ✗")
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(green).SetString("› ")
	t.Focused.SelectedOption = lipgloss.NewStyle()
	t.Focused.UnselectedOption = lipgloss.NewStyle()
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(green)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(green)
	t.Focused.TextInput.Placeholder = mutedStyle
	t.Blurred = t.Focused
	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}

func newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(formTheme)).WithShowHelp(false)
}

// yesNo 是/否问题也用列表而不是按钮，value 的初始值决定光标停在哪一项
func yesNo(title, desc, yes, no string, value *bool) *huh.Form {
	return newForm(huh.NewGroup(
		huh.NewSelect[bool]().Title(title).Description(desc).
			Options(huh.NewOption(yes, true), huh.NewOption(no, false)).
			Value(value),
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
	return strings.Join(parts, mutedStyle.Render(" · "))
}

// note 带标记的提示块：第一行是标记加标题，其余行缩进到标题下面
func note(marker, title string, lines ...string) string {
	var b strings.Builder
	b.WriteString(marker + " " + title)
	for _, l := range lines {
		b.WriteString("\n  " + l)
	}
	return b.String()
}

// reason 出错原因：按宽度折行后整体缩进两列，跟在所属行的下面
func reason(style lipgloss.Style, msg string, width int) string {
	wrapped := style.Width(width - 2).Render(strings.TrimSpace(msg))
	return "  " + strings.ReplaceAll(wrapped, "\n", "\n  ")
}

// table 两列对齐的信息表，标签灰色
func table(rows [][2]string) string {
	width := 0
	for _, r := range rows {
		width = max(width, lipgloss.Width(r[0]))
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, mutedStyle.Render(r[0]+strings.Repeat(" ", width-lipgloss.Width(r[0])))+"  "+r[1])
	}
	return strings.Join(lines, "\n")
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
