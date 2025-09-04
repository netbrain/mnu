package runner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type desktopItem struct {
	Name string
	Exec string // raw Exec command string (with field codes possibly removed)
}

func (d desktopItem) Title() string       { return d.Name }
func (d desktopItem) Description() string { return d.Exec }
func (d desktopItem) FilterValue() string { return strings.ToLower(d.Name + " " + d.Exec) }

type DesktopModel struct {
	width  int
	height int

	search textinput.Model
	list   list.Model

	all []desktopItem
	vis []desktopItem

	selectedExec string
}

// SelectedExec returns the Exec command of the selected desktop entry when Quit.
func (m DesktopModel) SelectedExec() string { return m.selectedExec }

// InitialDesktopModel returns a runner model populated from freedesktop desktop entries.
func InitialDesktopModel() tea.Model {
	si := textinput.New()
	si.Placeholder = "Search desktop apps…"
	si.Prompt = "/ "
	si.Focus()

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	all := discoverDesktopEntries()
	items := make([]list.Item, len(all))
	for i := range all { items[i] = all[i] }
	l.SetItems(items)

	vis := make([]desktopItem, len(all))
	copy(vis, all)
	return DesktopModel{search: si, list: l, all: all, vis: vis}
}

func (m DesktopModel) Init() tea.Cmd { return nil }

func (m DesktopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentWidth := m.width
		if contentWidth < 1 { contentWidth = 1 }
		m.search.Width = contentWidth - len(m.search.Prompt)
		m.list.SetSize(m.width, max(m.list.Height(), max(5, m.height-4)))
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
				for i := range m.vis { li[i] = m.vis[i] }
				m.list.SetItems(li)
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEnter:
			if it, ok := m.list.SelectedItem().(desktopItem); ok {
				m.selectedExec = it.Exec
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		q := strings.ToLower(strings.TrimSpace(m.search.Value()))
		if q == "" {
			vis := make([]desktopItem, len(m.all))
			copy(vis, m.all)
			m.vis = vis
		} else {
			vis := make([]desktopItem, 0, len(m.all))
			for _, it := range m.all {
				if strings.Contains(strings.ToLower(it.Name+" "+it.Exec), q) {
					vis = append(vis, it)
				}
			}
			m.vis = vis
		}
		li := make([]list.Item, len(m.vis))
		for i := range m.vis { li[i] = m.vis[i] }
		m.list.SetItems(li)
		if isListNavKey(msg) {
			m.list, _ = m.list.Update(msg)
		}
		return m, cmd
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m DesktopModel) View() string {
	return m.search.View()+"\n\n" + m.list.View()
}

// discoverDesktopEntries scans .desktop files per XDG Base Directory spec only.
// Directories scanned:
//   - $XDG_DATA_HOME/applications (default: ~/.local/share/applications)
//   - each $XDG_DATA_DIRS/applications (default: /usr/local/share:/usr/share)
func discoverDesktopEntries() []desktopItem {
	var dirs []string
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		dataHome = filepath.Join(os.Getenv("HOME"), ".local/share")
	}
	dirs = append(dirs, filepath.Join(dataHome, "applications"))
	dataDirs := strings.TrimSpace(os.Getenv("XDG_DATA_DIRS"))
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	for _, d := range strings.Split(dataDirs, ":") {
		d = strings.TrimSpace(d)
		if d == "" { continue }
		dirs = append(dirs, filepath.Join(d, "applications"))
	}

	seenFiles := map[string]struct{}{}
	var out []desktopItem
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil { continue }
		for _, de := range entries {
			if de.IsDir() { continue }
			name := de.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".desktop") { continue }
			full := filepath.Join(dir, name)
			if _, ok := seenFiles[strings.ToLower(full)]; ok { continue }
			if it, ok := parseDesktopFile(full); ok {
				seenFiles[strings.ToLower(full)] = struct{}{}
				out = append(out, it)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// parseDesktopFile extracts Name (localized) and Exec from the [Desktop Entry] section, applying basic filters.
func parseDesktopFile(path string) (desktopItem, bool) {
	data, err := os.ReadFile(path)
	if err != nil { return desktopItem{}, false }
	lines := strings.Split(string(data), "\n")
	inSection := false
	nameBase := ""
	nameLocales := map[string]string{}
	execLine := ""
	noDisplay := false
	hidden := false
	typeApp := false
	for _, ln := range lines {
		l := strings.TrimSpace(ln)
		if l == "" || strings.HasPrefix(l, "#") { continue }
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			inSection = strings.EqualFold(l, "[Desktop Entry]")
			continue
		}
		if !inSection { continue }
		if i := strings.Index(l, "="); i > 0 {
			k := strings.TrimSpace(l[:i])
			v := strings.TrimSpace(l[i+1:])
			// Handle localized Name fields: Name[xx] or Name[xx_YY]
			if strings.HasPrefix(k, "Name[") && strings.HasSuffix(k, "]") {
				loc := strings.TrimSuffix(strings.TrimPrefix(k, "Name["), "]")
				nameLocales[strings.ToLower(loc)] = v
				continue
			}
			switch k {
			case "Type":
				typeApp = strings.EqualFold(v, "Application")
			case "Name":
				if nameBase == "" { nameBase = v }
			case "TryExec":
				// optional: ignore for now
			case "Exec":
				execLine = v
			case "NoDisplay":
				noDisplay = strings.EqualFold(v, "true")
			case "Hidden":
				hidden = strings.EqualFold(v, "true")
			}
		}
	}
	// Choose best localized name
	name := chooseLocalizedName(nameBase, nameLocales)
	if !typeApp || hidden || noDisplay || execLine == "" || name == "" { return desktopItem{}, false }
	// Remove field codes like %f, %F, %u, %U, %i, %c, %k per spec. For our purposes, strip them.
	replacer := strings.NewReplacer("%f", "", "%F", "", "%u", "", "%U", "", "%i", "", "%c", "", "%k", "", "%%", "%")
	execClean := strings.TrimSpace(replacer.Replace(execLine))
	return desktopItem{Name: name, Exec: execClean}, true
}

// chooseLocalizedName selects the best Name given locale env vars.
func chooseLocalizedName(base string, locales map[string]string) string {
	if len(locales) == 0 { return base }
	// Build preference order from LC_MESSAGES or LANG
	pref := []string{}
	if v := strings.TrimSpace(os.Getenv("LC_MESSAGES")); v != "" { pref = append(pref, v) }
	if v := strings.TrimSpace(os.Getenv("LANG")); v != "" { pref = append(pref, v) }
	// Normalize variants: strip encoding (@variant and .charset), include language only
	norms := []string{}
	for _, p := range pref {
		p1 := p
		if i := strings.IndexAny(p1, ".@"); i >= 0 { p1 = p1[:i] }
		norms = append(norms, p1)
		if i := strings.Index(p1, "_"); i > 0 { norms = append(norms, p1[:i]) } else if len(p1) >= 2 { norms = append(norms, p1[:2]) }
	}
	// Lowercase keys for matching
	for _, k := range append(pref, norms...) {
		if v, ok := locales[strings.ToLower(k)]; ok && strings.TrimSpace(v) != "" { return v }
	}
	// Fallback to any provided locale if base is empty
	if strings.TrimSpace(base) == "" {
		for _, v := range locales { if strings.TrimSpace(v) != "" { return v } }
	}
	return base
}

