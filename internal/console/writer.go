package console

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

var (
	rootStyle      = lipgloss.NewStyle().PaddingLeft(1)
	primaryStyle   = rootStyle.Copy().Bold(true)
	secondaryStyle = rootStyle.Copy().Foreground(lipgloss.Color("#999999"))
	errorStyle     = rootStyle.Copy().Bold(true).Foreground(lipgloss.Color("#FF0000"))
)

func Fatalf(exitCode int, format string, a ...interface{}) {
	println(errorStyle, format, a...)
	os.Exit(exitCode)
}

func Secondaryf(format string, a ...interface{}) {
	println(secondaryStyle, format, a...)
}

func Primaryf(format string, a ...interface{}) {
	println(primaryStyle, format, a...)
}

func println(style lipgloss.Style, format string, a ...interface{}) {
	fmt.Println(style.Render(fmt.Sprintf(format, a...)))
}
