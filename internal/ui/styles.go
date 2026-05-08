package ui

import "charm.land/lipgloss/v2"

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
			Foreground(lipgloss.Green).
			MarginBottom(1)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Green).
			MarginBottom(1)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Green).
			Bold(true)

	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Yellow).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Red).
			Bold(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightBlack)

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.BrightBlack).
			Padding(1, 2)

	WarningBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Yellow).
			Padding(1, 2)

	ErrorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Red).
			Padding(1, 2)

	InfoBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Green).
			Padding(1, 2)

	LogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightBlack).
			PaddingLeft(4)

	KeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.White).
			Background(lipgloss.BrightBlack).
			Padding(0, 1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightBlack).
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
