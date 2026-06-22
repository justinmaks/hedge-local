package tui

import (
	"strings"
)

func RenderTabBar(active int, theme *Theme) string {
	var tabs []string
	for i, name := range tabNames {
		marker := "○ "
		style := theme.InactiveTab
		if i == active {
			marker = "◉ "
			style = theme.ActiveTab
		}
		tabs = append(tabs, style.Render(marker+name))
	}
	return strings.Join(tabs, " ")
}

func TabBarHeight() int {
	return 1
}
