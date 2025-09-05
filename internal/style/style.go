package style

import (
    "github.com/charmbracelet/bubbles/list"
    "github.com/charmbracelet/lipgloss"
)

// DocStyle is the common outer margin used by all TUIs (top/bottom=1, left/right=2).
var DocStyle = lipgloss.NewStyle().Margin(1, 2)

var (
    // Selected item styling matches mnu-bw: slight left padding and accent color.
    selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
)

// NewListDelegate returns a default list delegate with selected styles applied.
func NewListDelegate() list.DefaultDelegate {
    d := list.NewDefaultDelegate()
    d.Styles.SelectedTitle = selectedItemStyle
    d.Styles.SelectedDesc = selectedItemStyle.Copy().Faint(true)
    return d
}

// NewActionsDelegate returns a compact, single-line delegate for action menus.
func NewActionsDelegate() list.DefaultDelegate {
    d := list.NewDefaultDelegate()
    d.SetHeight(1)
    d.SetSpacing(0)
    d.Styles.SelectedTitle = selectedItemStyle
    d.Styles.SelectedDesc = selectedItemStyle
    return d
}

