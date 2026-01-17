package ui

import "github.com/charmbracelet/lipgloss"

// 颜色定义
var (
	ColorPrimary   = lipgloss.Color("#18a058") // 绿色主色
	ColorSecondary = lipgloss.Color("#36ad6a") // 浅绿色
	ColorSuccess   = lipgloss.Color("#18a058") // 绿色成功
	ColorWarning   = lipgloss.Color("#f0a020") // 橙黄色警告
	ColorError     = lipgloss.Color("#d03050") // 红色错误
	ColorMuted     = lipgloss.Color("#909399") // 灰色次要
	ColorHighlight = lipgloss.Color("#2080f0") // 蓝色高亮
)

// Logo ASCII艺术
const Logo = `
   █████╗  ██████╗███████╗██████╗  █████╗ ███╗   ██╗███████╗██╗
  ██╔══██╗██╔════╝██╔════╝██╔══██╗██╔══██╗████╗  ██║██╔════╝██║
  ███████║██║     █████╗  ██████╔╝███████║██╔██╗ ██║█████╗  ██║
  ██╔══██║██║     ██╔══╝  ██╔═══╝ ██╔══██║██║╚██╗██║██╔══╝  ██║
  ██║  ██║╚██████╗███████╗██║     ██║  ██║██║ ╚████║███████╗███████╗
  ╚═╝  ╚═╝ ╚═════╝╚══════╝╚═╝     ╚═╝  ╚═╝╚═╝  ╚═══╝╚══════╝╚══════╝
`

// 样式定义
var (
	LogoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Bold(true)

	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(1, 2)

	WarningBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorWarning).
			Padding(1, 2)

	ErrorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorError).
			Padding(1, 2)

	InfoBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	LogStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(4)

	KeyStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Background(lipgloss.Color("#374151")).
			Padding(0, 1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1)
)

func RenderLogo() string {
	return LogoStyle.Render(Logo)
}

func RenderTitle(title string) string {
	return TitleStyle.Render(title)
}

func RenderSuccess(msg string) string {
	return SuccessStyle.Render("✓ " + msg)
}

func RenderError(msg string) string {
	return ErrorStyle.Render("✗ " + msg)
}

func RenderWarning(msg string) string {
	return WarningStyle.Render("⚠ " + msg)
}

func RenderHelp(keys ...string) string {
	var result string
	for i := 0; i < len(keys); i += 2 {
		if i > 0 {
			result += "  "
		}
		if i+1 < len(keys) {
			result += KeyStyle.Render(keys[i]) + " " + MutedStyle.Render(keys[i+1])
		}
	}
	return HelpStyle.Render(result)
}
