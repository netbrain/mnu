package runner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type execItem struct {
	Name string
	Path string
}

func (e execItem) Title() string       { return e.Name }
func (e execItem) Description() string { return e.Path }
func (e execItem) FilterValue() string { return strings.ToLower(e.Name + " " + e.Path) }

type Model struct {
	width  int
	height int

	search textinput.Model
	list   list.Model

	all []execItem
	vis []execItem

	selectedPath string
}

// SelectedPath returns the absolute path of the selected executable when Quit.
func (m Model) SelectedPath() string { return m.selectedPath }

// InitialModel returns an app runner model populated from PATH.
func InitialModel() tea.Model {
	si := textinput.New()
	si.Placeholder = "Search commands…"
	si.Prompt = "/ "
	si.Focus()

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	all := discoverExecutables()
	items := make([]list.Item, len(all))
	for i := range all {
		items[i] = all[i]
	}
	l.SetItems(items)

	// Start with a separate copy for visible items to avoid aliasing issues
	vis := make([]execItem, len(all))
	copy(vis, all)
	return Model{search: si, list: l, all: all, vis: vis}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentWidth := m.width - docStyle.GetHorizontalFrameSize()
		if contentWidth < 1 {
			contentWidth = 1
		}
		m.search.Width = contentWidth - len(m.search.Prompt)
		m.list.SetSize(m.width, max(5, m.height-4))
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			if strings.TrimSpace(m.search.Value()) != "" {
				m.search.SetValue("")
				m.vis = m.all
				li := make([]list.Item, len(m.vis))
				for i := range m.vis {
					li[i] = m.vis[i]
				}
				m.list.SetItems(li)
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEnter:
			if it, ok := m.list.SelectedItem().(execItem); ok {
				m.selectedPath = it.Path
				return m, tea.Quit
			}
		}
		// update search then refilter; forward nav keys to list
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		q := strings.ToLower(strings.TrimSpace(m.search.Value()))
		if q == "" {
			// Show all; keep vis as an independent slice to avoid aliasing
			vis := make([]execItem, len(m.all))
			copy(vis, m.all)
			m.vis = vis
		} else {
			// Build a fresh filtered slice
			vis := make([]execItem, 0, len(m.all))
			for _, it := range m.all {
				if strings.Contains(strings.ToLower(it.Name+" "+it.Path), q) {
					vis = append(vis, it)
				}
			}
			m.vis = vis
		}
		li := make([]list.Item, len(m.vis))
		for i := range m.vis {
			li[i] = m.vis[i]
		}
		m.list.SetItems(li)
		// only navigation goes to list
		if isListNavKey(msg) {
			m.list, _ = m.list.Update(msg)
		}
		return m, cmd
	}
	// default: forward to list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	return docStyle.Render(m.search.View() + "\n\n" + m.list.View())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// isListNavKey duplicated to avoid importing ui package
func isListNavKey(k tea.KeyMsg) bool {
	switch k.Type {
	case tea.KeyUp, tea.KeyDown, tea.KeyRight, tea.KeyLeft,
		tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown,
		tea.KeyCtrlN, tea.KeyCtrlP, tea.KeyCtrlJ, tea.KeyCtrlK,
		tea.KeyTab, tea.KeyShiftTab:
		return true
	}
	return false
}

func discoverExecutables() []execItem {
	// seen by name (lowercased) and by canonical target path
	seenNames := map[string]struct{}{}
	seenTargets := map[string]struct{}{}
	var out []execItem
	paths := strings.Split(os.Getenv("PATH"), ":")
	for _, dir := range paths {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range entries {
			if de.IsDir() {
				continue
			}
			name := de.Name()
			// Skip hidden files to reduce noise
			if strings.HasPrefix(name, ".") {
				continue
			}
			full := filepath.Join(dir, name)
			// Follow symlinks and check target mode; this catches Nix-style wrappers
			info, err := os.Stat(full)
			if err != nil {
				continue
			}
			if info.Mode()&0111 == 0 {
				continue
			}
			// Resolve symlinks to a canonical target path if possible
			target := full
			if resolved, err := filepath.EvalSymlinks(full); err == nil && resolved != "" {
				target = resolved
			}
			lower := strings.ToLower(name)
			if _, ok := seenNames[lower]; ok {
				continue
			}
			if _, ok := seenTargets[target]; ok {
				continue
			}
			seenNames[lower] = struct{}{}
			seenTargets[target] = struct{}{}
			out = append(out, execItem{Name: name, Path: full})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
