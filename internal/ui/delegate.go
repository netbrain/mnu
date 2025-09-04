package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
)

func newDelegate() list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedItemStyle
	delegate.Styles.SelectedDesc = selectedItemStyle.Copy().Faint(true)
	return delegate
}

// newActionsDelegate creates a compact single-line delegate for the
// Password/Username/OTP menu to avoid extra whitespace and pagination.
func newActionsDelegate() list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	// Force one line per item and no extra spacing
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	// Minimal styling
	delegate.Styles.SelectedTitle = selectedItemStyle
	delegate.Styles.SelectedDesc = selectedItemStyle // not used but set
	return delegate
}
