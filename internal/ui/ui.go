package ui

import (
    "fmt"
    "strings"
    "time"

    "github.com/charmbracelet/bubbles/list"
    "github.com/charmbracelet/bubbles/textinput"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    cfgpkg "github.com/netbrain/bwmenu/internal/config"
    "github.com/netbrain/bwmenu/internal/clipboard"
    bwpkg "github.com/netbrain/bwmenu/internal/bw"
)


var docStyle = lipgloss.NewStyle().Margin(1, 2)

// UI states
const (
    stateCheckingLogin = iota
    stateUnlockPrompt
    stateLoadingItems
    stateList
    stateActionMenu
    stateCopying
    stateDone
)

type viewState int

type bwListItem struct {
    id          string
    title       string
    username    string
    desc        string
    hasTotp     bool
    url         string
    hasURL      bool
    hasUsername bool
    hasPassword bool
}

func (i bwListItem) Title() string       { return i.title }
func (i bwListItem) Description() string { return i.desc }
func (i bwListItem) FilterValue() string { return strings.ToLower(i.title + " " + i.desc) }

type actionItem struct {
    label string
    kind  string // "password" or "otp"
}

func (a actionItem) Title() string       { return a.label }
func (a actionItem) Description() string { return "" }
func (a actionItem) FilterValue() string { return a.label }

type model struct {
    manager bwpkg.Manager
    cfg     *cfgpkg.Config

    state   viewState
    width   int
    height  int

    // login
    password textinput.Model

    // list browsing
    allItems      []bwListItem
    visibleItems  []bwListItem
    list          list.Model
    search        textinput.Model

    // action menu
    selected     bwListItem
    actions      list.Model

    // feedback
    status string
    err    error

    // copy indicator
    copiedKind   string // "password", "username", or "otp"
    copiedUntil  time.Time
    copiedItemID string // item for which the copy indicator is active
    copyGen      int // increments per copy to disambiguate timers
}

// messages

type loginStatusMsg struct {
    loggedIn bool
    err      error
}

type unlockResultMsg struct {
    session string
    err     error
}

type itemsLoadedMsg struct {
    items []bwListItem
    err   error
}

type copyResultMsg struct {
    kind   string
    itemID string
    err    error
}

type copyIndicatorClearMsg struct{ gen int }
type copyIndicatorTickMsg struct{ gen int }

// InitialModel constructs the UI model to be passed to tea.NewProgram.
func InitialModel(manager bwpkg.Manager, cfg *cfgpkg.Config) tea.Model {
    // password input
    pw := textinput.New()
    pw.Placeholder = "Enter your Bitwarden master password"
    pw.Prompt = "Password: "
    pw.EchoMode = textinput.EchoPassword
    pw.EchoCharacter = '•'
    pw.Focus()

    // search input
    si := textinput.New()
    si.Placeholder = "Search..."
    si.Prompt = "/ "

    // list placeholder
    l := list.New([]list.Item{}, newDelegate(), 0, 0)
    l.SetShowTitle(false)
    l.SetShowStatusBar(false)
    l.SetFilteringEnabled(false)
    l.SetShowHelp(false)

    // actions list
    act := list.New([]list.Item{}, newActionsDelegate(), 0, 0)
    act.SetShowTitle(false)
    act.SetShowStatusBar(false)
    act.SetFilteringEnabled(false)
    act.SetShowHelp(false)
    act.SetShowPagination(false)

    return model{
        manager: manager,
        cfg:     cfg,
        state:   stateCheckingLogin,
        password: pw,
        search:   si,
        list:     l,
        actions:  act,
    }
}

func (m model) Init() tea.Cmd {
    return checkLoginCmd(m.manager)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height

        // Compute content width inside the document frame
        contentWidth := m.width - docStyle.GetHorizontalFrameSize()
        if contentWidth < 1 {
            contentWidth = 1
        }
        // Set all text inputs to max available space (minus their prompt widths)
        pwWidth := contentWidth - lipgloss.Width(m.password.Prompt)
        if pwWidth < 1 {
            pwWidth = 1
        }
        m.password.Width = pwWidth

        searchWidth := contentWidth - lipgloss.Width(m.search.Prompt)
        if searchWidth < 1 {
            searchWidth = 1
        }
        m.search.Width = searchWidth

        // leave some rows for search/status
        if m.state == stateList {
            m.list.SetSize(m.width, max(5, m.height-4))
        } else if m.state == stateActionMenu {
            // Fit to exactly the number of actions (single-line items)
            m.actions.SetSize(m.width, max(1, len(m.actions.Items())))
        }
        return m, nil

    case loginStatusMsg:
        if msg.err != nil {
            m.err = msg.err
            m.status = fmt.Sprintf("Error checking login: %v", msg.err)
            m.state = stateUnlockPrompt
            return m, nil
        }
        if msg.loggedIn {
            m.state = stateLoadingItems
            return m, loadItemsCmd(m.manager)
        }
        // not logged in
        m.state = stateUnlockPrompt
        return m, nil

    case unlockResultMsg:
        if msg.err != nil {
            m.status = fmt.Sprintf("Unlock failed: %v", msg.err)
            // keep in prompt
            return m, nil
        }
        // success; load items
        m.password.SetValue("")
        m.state = stateLoadingItems
        return m, loadItemsCmd(m.manager)

    case itemsLoadedMsg:
        if msg.err != nil {
            m.status = fmt.Sprintf("Failed to load items: %v", msg.err)
            return m, nil
        }
        m.allItems = msg.items
        m.visibleItems = msg.items
        items := make([]list.Item, len(m.visibleItems))
        for i := range m.visibleItems {
            items[i] = m.visibleItems[i]
        }
        m.list.SetItems(items)
        m.state = stateList
        // Focus the search input so typing filters immediately
        m.password.Blur()
        m.search.Focus()
        if m.width > 0 && m.height > 0 {
            m.list.SetSize(m.width, max(5, m.height-4))
        }
        return m, nil

    case copyResultMsg:
        // After copying, show an icon and countdown until clipboard is cleared.
        if msg.err == nil {
            // bump generation to invalidate previous timers
            m.copyGen++
            gen := m.copyGen
            m.copiedKind = msg.kind
            m.copiedItemID = msg.itemID
            // Use configured clipboard timeout for the countdown
            m.copiedUntil = time.Now().Add(m.cfg.ClipboardTimeout)
            // Rebuild action items with indicator if we're in the action menu
            if m.state == stateActionMenu {
                m.actions.SetItems(m.buildActions())
            }
            // Schedule both periodic countdown updates and a final clear tied to this gen
            return m, tea.Batch(
                tea.Tick(time.Second, func(time.Time) tea.Msg { return copyIndicatorTickMsg{gen: gen} }),
                tea.Tick(time.Until(m.copiedUntil), func(time.Time) tea.Msg { return copyIndicatorClearMsg{gen: gen} }),
            )
        }
        return m, nil

    case copyIndicatorTickMsg:
        // Update countdown label; keep ticking until time elapsed
        if msg.gen != m.copyGen {
            return m, nil // stale tick
        }
        if !m.copiedUntil.IsZero() && time.Now().Before(m.copiedUntil) {
            if m.state == stateActionMenu {
                m.actions.SetItems(m.buildActions())
            }
            gen := m.copyGen
            return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return copyIndicatorTickMsg{gen: gen} })
        }
        return m, nil

    case copyIndicatorClearMsg:
        // Clear indicator and restore labels (only if current gen)
        if msg.gen != m.copyGen {
            return m, nil // stale clear
        }
        m.copiedKind = ""
        m.copiedUntil = time.Time{}
        m.copiedItemID = ""
        if m.state == stateActionMenu {
            m.actions.SetItems(m.buildActions())
        }
        return m, nil

    case tea.KeyMsg:
        switch m.state {
        case stateUnlockPrompt:
            switch msg.Type {
            case tea.KeyEnter:
                pw := m.password.Value()
                if pw == "" {
                    m.status = "Password cannot be empty"
                    return m, nil
                }
                return m, unlockCmd(m.manager, pw)
            case tea.KeyEsc, tea.KeyCtrlC:
                return m, tea.Quit
            default:
                var cmd tea.Cmd
                m.password, cmd = m.password.Update(msg)
                return m, cmd
            }

        case stateList:
            switch msg.Type {
            case tea.KeyCtrlC:
                return m, tea.Quit
            case tea.KeyEsc:
                // If there is an active search filter, clear it instead of quitting.
                if strings.TrimSpace(m.search.Value()) != "" {
                    m.search.SetValue("")
                    m.visibleItems = m.allItems
                    li := make([]list.Item, len(m.visibleItems))
                    for i := range m.visibleItems {
                        li[i] = m.visibleItems[i]
                    }
                    m.list.SetItems(li)
                    return m, nil
                }
                // No filter active: quit the app.
                return m, tea.Quit
            case tea.KeyEnter:
                if itm, ok := m.list.SelectedItem().(bwListItem); ok {
                    m.selected = itm
                    // build actions in order: Password, Username, OTP (if available)
                    acts := []list.Item{actionItem{label: "Password", kind: "password"}, actionItem{label: "Username", kind: "username"}}
                    if itm.hasTotp {
                        acts = append(acts, actionItem{label: "OTP", kind: "otp"})
                    }
                    // set actions list and preselect Password (index 0)
                    m.actions.SetItems(m.buildActions())
                    m.actions.Select(0)
                    // size to show exactly all actions (single-line)
                    if m.width > 0 {
                        m.actions.SetSize(m.width, max(1, len(m.actions.Items())))
                    }
                    m.state = stateActionMenu
                    return m, nil
                }
            default:
                // Update search input first (it's focused)
                var cmd tea.Cmd
                m.search, cmd = m.search.Update(msg)
                // Re-filter based on search query
                q := strings.ToLower(strings.TrimSpace(m.search.Value()))
                if q == "" {
                    m.visibleItems = m.allItems
                } else {
                    m.visibleItems = make([]bwListItem, 0, len(m.allItems))
                    for _, it := range m.allItems {
                        if strings.Contains(strings.ToLower(it.title+" "+it.desc), q) {
                            m.visibleItems = append(m.visibleItems, it)
                        }
                    }
                }
                li := make([]list.Item, len(m.visibleItems))
                for i := range m.visibleItems {
                    li[i] = m.visibleItems[i]
                }
                m.list.SetItems(li)
                // Forward only navigation keys to the list so typing doesn't get eaten
                if isListNavKey(msg) {
                    m.list, _ = m.list.Update(msg)
                }
                return m, cmd
            }

        case stateActionMenu:
            switch msg.Type {
            case tea.KeyCtrlC:
                return m, tea.Quit
            case tea.KeyEsc:
                m.state = stateList
                return m, nil
case tea.KeyEnter:
                if it, ok := m.actions.SelectedItem().(actionItem); ok {
                    return m, m.copyCmd(it.kind, m.selected.id, m.selected.username)
                }
                return m, nil
            default:
                // only forward navigation keys to the actions list
                if isListNavKey(msg) {
                    var cmd tea.Cmd
                    m.actions, cmd = m.actions.Update(msg)
                    return m, cmd
                }
                return m, nil
            }
        }
    }

    // Default: allow controls to update input/list where relevant
    switch m.state {
    case stateUnlockPrompt:
        var cmd tea.Cmd
        m.password, cmd = m.password.Update(msg)
        return m, cmd
    case stateList:
        var cmd tea.Cmd
        m.list, cmd = m.list.Update(msg)
        return m, cmd
    case stateActionMenu:
        // handled in the main switch above to filter keys; do nothing here
        return m, nil
    }

    return m, nil
}

func (m model) View() string {
    switch m.state {
    case stateCheckingLogin:
        return docStyle.Render("Checking login status…")
    case stateUnlockPrompt:
        return docStyle.Render(m.password.View())
    case stateLoadingItems:
        return docStyle.Render("Loading items…")
    case stateList:
        return docStyle.Render(m.search.View()+"\n\n" + m.list.View())
    case stateActionMenu:
        return docStyle.Render("Selected: " + m.selected.title + "\n" + m.actions.View())
    case stateDone:
        return docStyle.Render("")
    default:
        return ""
    }
}

// Commands and helpers

func checkLoginCmd(mgr bwpkg.Manager) tea.Cmd {
    return func() tea.Msg {
        logged, err := mgr.IsLoggedIn()
        return loginStatusMsg{loggedIn: logged, err: err}
    }
}

func unlockCmd(mgr bwpkg.Manager, password string) tea.Cmd {
    return func() tea.Msg {
        session, err := mgr.Unlock(password)
        return unlockResultMsg{session: session, err: err}
    }
}

func loadItemsCmd(mgr bwpkg.Manager) tea.Cmd {
    return func() tea.Msg {
        raw, err := mgr.GetItems()
        if err != nil {
            return itemsLoadedMsg{nil, err}
        }
        items := make([]bwListItem, 0, len(raw))
        for _, r := range raw {
            items = append(items, bwListItemFromMap(r))
        }
        return itemsLoadedMsg{items: items, err: nil}
    }
}

func (m model) buildActions() []list.Item {
    // Build actions with optional indicator icons appended
    items := []list.Item{}
    now := time.Now()
    showIndicator := m.copiedKind != "" && now.Before(m.copiedUntil) && m.selected.id == m.copiedItemID
    // Helper to append remaining seconds if needed
    appendCountdown := func(lbl, kind string) string {
        if !showIndicator || m.copiedKind != kind { return lbl }
        rem := time.Until(m.copiedUntil)
        if rem < 0 { return lbl }
        // ceil to seconds
        secs := int((rem + time.Second - 1) / time.Second)
        return fmt.Sprintf("%s %ds", lbl, secs)
    }

    // Password (only if present)
    if m.selected.hasPassword {
        base := "Password"
        if showIndicator && m.copiedKind == "password" {
            base = "🔑 " + base
        }
        base = appendCountdown(base, "password")
        items = append(items, actionItem{label: base, kind: "password"})
    }

    // Username (only if present)
    if m.selected.hasUsername {
        base := "Username"
        if showIndicator && m.copiedKind == "username" {
            base = "👤 " + base
        }
        base = appendCountdown(base, "username")
        items = append(items, actionItem{label: base, kind: "username"})
    }

    // URL if available
    if m.selected.hasURL {
        base := "URL"
        if showIndicator && m.copiedKind == "url" {
            base = "🌐 " + base
        }
        base = appendCountdown(base, "url")
        items = append(items, actionItem{label: base, kind: "url"})
    }

    // OTP next if available
    if m.selected.hasTotp {
        base := "OTP"
        if showIndicator && m.copiedKind == "otp" {
            base = "🕒 " + base
        }
        base = appendCountdown(base, "otp")
        items = append(items, actionItem{label: base, kind: "otp"})
    }
    return items
}

func (m model) copyCmd(kind, id, username string) tea.Cmd {
    return func() tea.Msg {
        var secret string
        var err error
        switch kind {
        case "password":
            if !m.selected.hasPassword {
                err = fmt.Errorf("no password for this item")
                break
            }
            secret, err = m.manager.GetPassword(id)
        case "otp":
            secret, err = m.manager.GetTotp(id)
        case "username":
            if !m.selected.hasUsername {
                err = fmt.Errorf("no username for this item")
                break
            }
            secret = username
        case "url":
            if !m.selected.hasURL {
                err = fmt.Errorf("no URL for this item")
                break
            }
            secret = m.selected.url
        default:
            err = fmt.Errorf("unknown copy kind: %s", kind)
        }
        if err != nil {
            return copyResultMsg{kind: kind, itemID: id, err: err}
        }
        trimmed := strings.TrimSpace(secret)
        // Convert to bytes and clear the string variable to reduce exposure
        b := []byte(trimmed)
        secret = ""
        trimmed = ""
        if err := clipboard.CopyBytes(b, m.cfg.ClipboardTimeout); err != nil {
            return copyResultMsg{kind: kind, itemID: id, err: err}
        }
        return copyResultMsg{kind: kind, itemID: id, err: nil}
    }
}

func str(v interface{}) string {
    if v == nil { return "" }
    switch t := v.(type) {
    case string:
        return t
    default:
        return fmt.Sprintf("%v", v)
    }
}

func getMap(m map[string]interface{}, keys ...string) map[string]interface{} {
    cur := m
    for _, k := range keys {
        v, ok := cur[k]
        if !ok {
            return nil
        }
        mv, ok := v.(map[string]interface{})
        if !ok {
            return nil
        }
        cur = mv
    }
    return cur
}

func getStringDeep(m map[string]interface{}, keys ...string) string {
    if len(keys) == 0 { return "" }
    // try top-level
    if v, ok := m[keys[0]]; ok && len(keys) == 1 {
        return str(v)
    }
    // try deep path
    mm := getMap(m, keys[:len(keys)-1]...)
    if mm == nil { return "" }
    return str(mm[keys[len(keys)-1]])
}

func bwListItemFromMap(m map[string]interface{}) bwListItem {
    id := getStringDeep(m, "id")
    if id == "" {
        id = getStringDeep(m, "data", "id")
    }
    title := getStringDeep(m, "name")
    if title == "" {
        title = getStringDeep(m, "data", "name")
    }
    if title == "" {
        title = "(no title)"
    }
    username := getStringDeep(m, "login", "username")
    if username == "" {
        username = getStringDeep(m, "data", "login", "username")
    }
    hasUsername := strings.TrimSpace(username) != ""
    desc := username
    hasTotp := false
    totp := getStringDeep(m, "login", "totp")
    if totp == "" {
        totp = getStringDeep(m, "data", "login", "totp")
    }
    if strings.TrimSpace(totp) != "" {
        hasTotp = true
    }
    // Extract first URI if present
    url := firstURIFromItem(m)
    hasURL := strings.TrimSpace(url) != ""

    // Detect password presence without storing it
    pw := getStringDeep(m, "login", "password")
    if pw == "" {
        pw = getStringDeep(m, "data", "login", "password")
    }
    hasPassword := strings.TrimSpace(pw) != ""

    return bwListItem{
        id:          id,
        title:       title,
        username:    username,
        desc:        desc,
        hasTotp:     hasTotp,
        url:         url,
        hasURL:      hasURL,
        hasUsername: hasUsername,
        hasPassword: hasPassword,
    }
}

func max(a, b int) int { if a > b { return a }; return b }

// firstURIFromItem tries to find the first URL/URI from common Bitwarden item shapes.
func firstURIFromItem(m map[string]interface{}) string {
    // Try login.uris (array of objects with uri field)
    if login, ok := m["login"].(map[string]interface{}); ok {
        if s := firstURIFromLogin(login); s != "" { return s }
    }
    // Try data.login.uris
    if data, ok := m["data"].(map[string]interface{}); ok {
        if login, ok := data["login"].(map[string]interface{}); ok {
            if s := firstURIFromLogin(login); s != "" { return s }
        }
    }
    // Fall back to possible direct fields
    if s := strings.TrimSpace(getStringDeep(m, "login", "uri")); s != "" { return s }
    if s := strings.TrimSpace(getStringDeep(m, "data", "login", "uri")); s != "" { return s }
    return ""
}

func firstURIFromLogin(login map[string]interface{}) string {
    if v, ok := login["uris"]; ok {
        switch vv := v.(type) {
        case []interface{}:
            for _, it := range vv {
                // item can be a map with uri key, or a string
                if mp, ok := it.(map[string]interface{}); ok {
                    s := strings.TrimSpace(str(mp["uri"]))
                    if s != "" { return s }
                } else if s, ok := it.(string); ok {
                    s = strings.TrimSpace(s)
                    if s != "" { return s }
                }
            }
        case map[string]interface{}:
            s := strings.TrimSpace(str(vv["uri"]))
            if s != "" { return s }
        case string:
            s := strings.TrimSpace(vv)
            if s != "" { return s }
        }
    }
    return ""
}

// isListNavKey returns true if the key should be handled by the list for navigation.
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

