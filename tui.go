package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// ---------- Estilos ----------

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#5B3FD6", Dark: "#A78BFA"}
	cOK     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	cBad    = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	cWarn   = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	cDim    = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	cUser   = lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#93C5FD"}
	cBtnBg  = lipgloss.AdaptiveColor{Light: "#E5E7EB", Dark: "#374151"}
	cBtnFg  = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F9FAFB"}

	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sTab    = lipgloss.NewStyle().Padding(0, 1).Foreground(cDim)
	sTabOn  = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sOK     = lipgloss.NewStyle().Foreground(cOK)
	sBad    = lipgloss.NewStyle().Foreground(cBad)
	sWarn   = lipgloss.NewStyle().Foreground(cWarn)
	sBold   = lipgloss.NewStyle().Bold(true)
	sSel    = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sUser   = lipgloss.NewStyle().Bold(true).Foreground(cUser)
	sBot    = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cAccent).Padding(0, 1)
	sCard   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDim).Padding(0, 1)
	sCardOn = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cAccent).Padding(0, 1)
	sHelp   = lipgloss.NewStyle().Foreground(cDim)
	sBtn    = lipgloss.NewStyle().Padding(0, 1).Foreground(cBtnFg).Background(cBtnBg)
	sBtnPri = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent)
	sBtnBad = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#FFFFFF")).Background(cBad)
)

var tabNames = []string{"Inicio", "Chat", "Archivos", "Modelos", "Guías", "Ajustes"}
var tabShort = []string{"Inicio", "Chat", "Archivos", "Modelos", "Guías", "Ajustes"}

const (
	tabHome = iota
	tabChat
	tabFiles
	tabModels
	tabGuides
	tabSettings
)

// ---------- Mensajes ----------

type tickMsg time.Time
type statusMsg struct{}
type modelsMsg struct {
	list []string
	err  error
}
type chatEvMsg struct {
	ev chatEvent
	ch <-chan chatEvent
}
type mediaDoneMsg struct {
	kind string
	err  error
}
type flashMsg string

// ---------- Botones y zonas clicables ----------

// btn dibuja un botón clicable. El id es la acción que se ejecuta al pulsarlo.
func (m *model) btn(id, label string, st lipgloss.Style) string {
	m.zones = append(m.zones, id)
	return zone.Mark(id, st.Render(label))
}

func (m *model) mark(id, s string) string {
	m.zones = append(m.zones, id)
	return zone.Mark(id, s)
}

// row coloca botones en líneas que caben en el ancho disponible.
func row(width int, items ...string) string {
	var lines []string
	cur := ""
	for _, it := range items {
		if it == "" {
			continue
		}
		if cur == "" {
			cur = it
			continue
		}
		if lipgloss.Width(cur)+1+lipgloss.Width(it) > width {
			lines = append(lines, cur)
			cur = it
		} else {
			cur += " " + it
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

// ---------- Formulario genérico ----------

type form struct {
	id     string
	title  string
	labels []string
	inputs []textinput.Model
	focus  int
}

func newForm(id, title string, labels, values []string, secret []bool, width int) *form {
	f := &form{id: id, title: title, labels: labels}
	for i := range labels {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 4096
		ti.Width = max(16, width-8)
		ti.SetValue(values[i])
		if secret != nil && secret[i] {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		}
		f.inputs = append(f.inputs, ti)
	}
	f.inputs[0].Focus()
	return f
}

func (f *form) values() []string {
	out := make([]string, len(f.inputs))
	for i, in := range f.inputs {
		out[i] = strings.TrimSpace(in.Value())
	}
	return out
}

func (f *form) setFocus(i int) {
	f.inputs[f.focus].Blur()
	f.focus = (i + len(f.inputs)) % len(f.inputs)
	f.inputs[f.focus].Focus()
}

func (f *form) move(d int) { f.setFocus(f.focus + d) }

func (f *form) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd
}

func (f *form) toggleSecret() {
	for i := range f.inputs {
		switch {
		case f.inputs[i].EchoMode == textinput.EchoPassword:
			f.inputs[i].EchoMode = textinput.EchoNormal
			f.inputs[i].EchoCharacter = 0x2007 // marca: era secreto
		case f.inputs[i].EchoCharacter == 0x2007:
			f.inputs[i].EchoMode = textinput.EchoPassword
			f.inputs[i].EchoCharacter = '•'
		}
	}
}

// Etiqueta encima del campo: cabe en pantallas estrechas (móvil).
func (m *model) formView(f *form, height int) string {
	var sb strings.Builder
	if f.title != "" {
		sb.WriteString(sTitle.Render(f.title) + "\n")
	}
	per := 2
	first := 0
	maxFields := max(1, (height-4)/per)
	if f.focus >= maxFields {
		first = f.focus - maxFields + 1
	}
	for i := first; i < len(f.labels) && i < first+maxFields; i++ {
		l := f.labels[i]
		if i == f.focus {
			sb.WriteString(m.mark(fmt.Sprintf("%s:field:%d", f.id, i), sSel.Render("▸ "+l)) + "\n")
			sb.WriteString("  " + f.inputs[i].View() + "\n")
		} else {
			v := f.inputs[i].View()
			sb.WriteString(m.mark(fmt.Sprintf("%s:field:%d", f.id, i), sDim.Render("  "+l)+"\n  "+v) + "\n")
		}
	}
	sb.WriteString("\n" + row(m.w-2,
		m.btn(f.id+":save", "Guardar", sBtnPri),
		m.btn(f.id+":cancel", "Cancelar", sBtn),
		m.btn(f.id+":reveal", "Ver claves", sBtn),
	))
	return sb.String()
}

// ---------- Lista con filtro ----------

type pickList struct {
	id      string
	items   []string
	checked map[string]bool
	multi   bool
	cursor  int
	offset  int
	filter  textinput.Model
	filtOn  bool
}

func newPickList(id string, items []string, multi bool, checked []string) *pickList {
	ti := textinput.New()
	ti.Prompt = "Buscar: "
	ti.Placeholder = "escribe para filtrar"
	p := &pickList{id: id, items: items, multi: multi, checked: map[string]bool{}, filter: ti}
	for _, c := range checked {
		p.checked[c] = true
	}
	return p
}

func (p *pickList) visible() []string {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	if q == "" {
		return p.items
	}
	var out []string
	for _, it := range p.items {
		if strings.Contains(strings.ToLower(it), q) {
			out = append(out, it)
		}
	}
	return out
}

func (p *pickList) current() string {
	v := p.visible()
	if p.cursor >= 0 && p.cursor < len(v) {
		return v[p.cursor]
	}
	return ""
}

func (p *pickList) scroll(d int) {
	n := len(p.visible())
	p.cursor = min(max(0, n-1), max(0, p.cursor+d))
}

// handleKey devuelve true si consumió la tecla.
func (p *pickList) handleKey(k tea.KeyMsg, height int) (bool, tea.Cmd) {
	if p.filtOn {
		switch k.String() {
		case "esc", "enter":
			p.filtOn = false
			p.filter.Blur()
			return true, nil
		case "up", "down":
		default:
			var cmd tea.Cmd
			p.filter, cmd = p.filter.Update(k)
			p.cursor, p.offset = 0, 0
			return true, cmd
		}
	}
	n := len(p.visible())
	switch k.String() {
	case "/":
		p.filtOn = true
		p.filter.Focus()
		return true, textinput.Blink
	case "up", "k":
		p.scroll(-1)
	case "down", "j":
		p.scroll(1)
	case "pgup":
		p.scroll(-height)
	case "pgdown":
		p.scroll(height)
	case "home":
		p.cursor = 0
	case "end":
		p.cursor = max(0, n-1)
	case " ":
		if p.multi {
			if c := p.current(); c != "" {
				p.checked[c] = !p.checked[c]
			}
			return true, nil
		}
		return false, nil
	default:
		return false, nil
	}
	return true, nil
}

func (m *model) pickView(p *pickList, height int) string {
	v := p.visible()
	height = max(3, height)
	var sb strings.Builder
	sb.WriteString(m.mark(p.id+":filter", p.filter.View()) + "\n")
	height--
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+height {
		p.offset = p.cursor - height + 1
	}
	if len(v) == 0 {
		sb.WriteString(sDim.Render("  (vacío)") + "\n")
	}
	for i := p.offset; i < len(v) && i < p.offset+height; i++ {
		box := ""
		if p.multi {
			box = "[ ] "
			if p.checked[v[i]] {
				box = sOK.Render("[x] ")
			}
		}
		line := truncate(v[i], m.w-8)
		var s string
		if i == p.cursor {
			s = sSel.Render("▸ ") + box + sSel.Render(line)
		} else {
			s = "  " + box + line
		}
		sb.WriteString(m.mark(fmt.Sprintf("%s:row:%d", p.id, i), s) + "\n")
	}
	sb.WriteString(sDim.Render(fmt.Sprintf("  %d de %d", min(len(v), p.cursor+1), len(v))))
	return sb.String()
}

func truncate(s string, n int) string {
	if n < 4 {
		n = 4
	}
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// ---------- Modelo principal ----------

type model struct {
	cfg  *Config
	core *Core
	apps *AppManager
	w, h int
	tab  int

	zones     []string
	flash     string
	flashTime time.Time
	st        Status
	ticks     int

	// Registro de FreeLLMAPI
	logOpen bool
	logVP   viewport.Model

	// Chat
	vp        viewport.Model
	ta        textarea.Model
	partial   string
	streaming bool
	chatRoute string
	histLen   int

	// Archivos
	files      []FileInfo
	fcursor    int
	confirmDel string

	// Modelos
	mlist         *pickList
	modelsErr     string
	modelsLoading bool

	// Guías
	guides  []guide
	gcursor int
	gvp     viewport.Model

	// Ajustes
	sform *form
}

func newModel(cfg *Config, core *Core) *model {
	ta := textarea.New()
	ta.Placeholder = "Escribe tu mensaje… (Enter envía · Alt+Enter salto de línea)"
	ta.ShowLineNumbers = false
	ta.Prompt = "┃ "
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.KeyMap.InsertNewline.SetKeys("alt+enter", "ctrl+j")
	m := &model{
		cfg: cfg, core: core, apps: core.apps,
		ta:     ta,
		vp:     viewport.New(80, 20),
		gvp:    viewport.New(80, 20),
		logVP:  viewport.New(80, 20),
		guides: loadGuides(),
		tab:    tabChat,
	}
	m.st = core.Status()
	return m
}

func (m *model) Init() tea.Cmd {
	m.core.Boot()
	return tea.Batch(m.refresh(), tick(), tea.SetWindowTitle("Infinito"), m.ta.Focus())
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) refresh() tea.Cmd {
	return func() tea.Msg { m.core.Refresh(); return statusMsg{} }
}

func (m *model) setFlash(s string) {
	m.flash = s
	m.flashTime = time.Now()
}

func (m *model) loadModels() tea.Cmd {
	m.modelsLoading = true
	return func() tea.Msg {
		l, err := m.core.Models()
		return modelsMsg{list: l, err: err}
	}
}

func (m *model) inputActive() bool {
	switch {
	case m.tab == tabChat, m.tab == tabSettings:
		return true
	case m.tab == tabModels && m.mlist != nil && m.mlist.filtOn:
		return true
	}
	return false
}

func (m *model) switchTab(t int) tea.Cmd {
	m.tab = (t + len(tabNames)) % len(tabNames)
	m.confirmDel = ""
	var cmds []tea.Cmd
	if m.tab == tabChat {
		cmds = append(cmds, m.ta.Focus())
		m.renderChat()
	} else {
		m.ta.Blur()
	}
	switch m.tab {
	case tabModels:
		if m.mlist == nil {
			cmds = append(cmds, m.loadModels())
		}
	case tabFiles:
		m.files = ListFiles()
	case tabSettings:
		m.openSettings()
	case tabGuides:
		m.refreshGuide()
	case tabHome:
		cmds = append(cmds, m.refresh())
	}
	return tea.Batch(cmds...)
}

func (m *model) narrow() bool { return m.w < 72 }

func (m *model) layout() {
	bodyH := m.h - 4
	m.ta.SetWidth(max(10, m.w-2))
	m.vp.Width = m.w
	m.vp.Height = max(3, bodyH-m.ta.Height()-3)
	if m.narrow() {
		m.gvp.Width = m.w
		m.gvp.Height = max(3, bodyH-3)
	} else {
		m.gvp.Width = max(20, m.w-guideListW()-2)
		m.gvp.Height = max(3, bodyH-1)
	}
	m.logVP.Width = m.w - 2
	m.logVP.Height = max(3, bodyH-3)
	m.renderChat()
	m.refreshGuide()
}

func guideListW() int { return 30 }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tickMsg:
		if m.flash != "" && time.Since(m.flashTime) > 7*time.Second {
			m.flash = ""
		}
		if m.logOpen {
			m.refreshLog()
		}
		m.st = m.core.Status()
		// La conversación también cambia mientras se genera multimedia.
		if n := len(m.core.History()); n != m.histLen && !m.streaming {
			m.renderChat()
		}
		m.ticks++
		var cmds []tea.Cmd
		cmds = append(cmds, tick())
		if m.ticks%6 == 0 {
			cmds = append(cmds, m.refresh())
		}
		if m.tab == tabFiles && m.ticks%4 == 0 {
			m.files = ListFiles()
		}
		return m, tea.Batch(cmds...)

	case statusMsg:
		m.st = m.core.Status()
		return m, nil

	case flashMsg:
		m.setFlash(string(msg))
		return m, nil

	case modelsMsg:
		m.modelsLoading = false
		if msg.err != nil {
			m.modelsErr = msg.err.Error()
			m.mlist = nil
		} else {
			m.modelsErr = ""
			l := msg.list
			if !contains(l, "auto") {
				l = append([]string{"auto"}, l...)
			}
			m.mlist = newPickList("mlist", l, false, nil)
			for i, x := range l {
				if x == m.core.model() {
					m.mlist.cursor = i
				}
			}
		}
		return m, nil

	case chatEvMsg:
		return m, m.onChatEvent(msg)

	case mediaDoneMsg:
		if msg.err != nil {
			m.setFlash("No se pudo generar: " + short(msg.err.Error(), 120))
		} else {
			m.setFlash("Listo. Lo tienes en Archivos y en " + filesDir())
		}
		m.renderChat()
		return m, nil

	case tea.MouseMsg:
		return m, m.onMouse(msg)

	case tea.KeyMsg:
		return m, m.onKey(msg)
	}

	var cmds []tea.Cmd
	if m.tab == tabChat {
		var c tea.Cmd
		m.ta, c = m.ta.Update(msg)
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

// ---------- Ratón y pantalla táctil ----------

func (m *model) onMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		d := 3
		if msg.Button == tea.MouseButtonWheelUp {
			d = -3
		}
		return m.scroll(d)
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	// Recorremos al revés: lo dibujado al final queda por encima.
	for i := len(m.zones) - 1; i >= 0; i-- {
		id := m.zones[i]
		if z := zone.Get(id); z != nil && z.InBounds(msg) {
			return m.act(id)
		}
	}
	return nil
}

func (m *model) scroll(d int) tea.Cmd {
	switch {
	case m.logOpen:
		if d < 0 {
			m.logVP.LineUp(-d)
		} else {
			m.logVP.LineDown(d)
		}
	case m.tab == tabChat:
		if d < 0 {
			m.vp.LineUp(-d)
		} else {
			m.vp.LineDown(d)
		}
	case m.tab == tabGuides:
		if d < 0 {
			m.gvp.LineUp(-d)
		} else {
			m.gvp.LineDown(d)
		}
	case m.tab == tabModels && m.mlist != nil:
		m.mlist.scroll(d)
	case m.tab == tabFiles:
		m.fcursor = min(max(0, len(m.files)-1), max(0, m.fcursor+d/3))
	case m.tab == tabSettings && m.sform != nil:
		m.sform.move(d / 3)
	}
	return nil
}

// act ejecuta la acción de un botón o zona pulsada.
func (m *model) act(id string) tea.Cmd {
	parts := strings.SplitN(id, ":", 3)
	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	num := func(i int) int { n, _ := strconv.Atoi(arg(i)); return n }
	switch parts[0] {
	case "tab":
		return m.switchTab(num(1))
	case "quit":
		return m.quit()
	case "home":
		return m.homeAction(arg(1))
	case "chat":
		return m.chatAction(arg(1), arg(2))
	case "file":
		return m.fileAction(arg(1), arg(2))
	case "frow":
		m.fcursor = num(1)
		m.confirmDel = ""
	case "mlist":
		if m.mlist == nil {
			return nil
		}
		if arg(1) == "filter" {
			m.mlist.filtOn = true
			return m.mlist.filter.Focus()
		}
		if arg(1) == "row" {
			m.mlist.cursor = num(2)
		}
		return m.keyModels(tea.KeyMsg{Type: tea.KeyEnter})
	case "mact":
		switch arg(1) {
		case "reload":
			return m.keyModels(keyRune('r'))
		case "copy":
			return m.keyModels(keyRune('c'))
		case "filter":
			return m.act("mlist:filter")
		}
	case "sform":
		f := m.sform
		if f == nil {
			return nil
		}
		switch arg(1) {
		case "field":
			f.setFocus(num(2))
		case "save":
			return m.keySettings(tea.KeyMsg{Type: tea.KeyEnter})
		case "cancel":
			return m.keySettings(tea.KeyMsg{Type: tea.KeyEsc})
		case "reveal":
			f.toggleSecret()
		}
	case "guide":
		m.gcursor = num(1)
		m.refreshGuide()
	case "log":
		if arg(1) == "close" {
			m.logOpen = false
		}
	}
	return nil
}

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func (m *model) quit() tea.Cmd {
	m.core.Stop()
	return tea.Quit
}

func (m *model) onKey(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		return m.quit()
	}
	if m.logOpen {
		switch s {
		case "esc", "q", "enter":
			m.logOpen = false
			return nil
		}
		var c tea.Cmd
		m.logVP, c = m.logVP.Update(k)
		return c
	}
	if m.tab != tabSettings {
		if s == "tab" {
			return m.switchTab(m.tab + 1)
		}
		if s == "shift+tab" {
			return m.switchTab(m.tab - 1)
		}
	}
	if s == "ctrl+right" || s == "alt+right" {
		return m.switchTab(m.tab + 1)
	}
	if s == "ctrl+left" || s == "alt+left" {
		return m.switchTab(m.tab - 1)
	}
	if !m.inputActive() {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(tabNames) {
			return m.switchTab(n - 1)
		}
		if s == "q" {
			return m.quit()
		}
	}
	switch m.tab {
	case tabHome:
		return m.keyHome(s)
	case tabChat:
		return m.keyChat(k)
	case tabFiles:
		return m.keyFiles(s)
	case tabModels:
		return m.keyModels(k)
	case tabGuides:
		return m.keyGuides(k)
	case tabSettings:
		return m.keySettings(k)
	}
	return nil
}

// ---------- Vista ----------

func (m *model) View() string {
	if m.w == 0 {
		return "Cargando…"
	}
	m.zones = m.zones[:0]
	names := tabNames
	if m.w < 96 {
		names = tabShort
	}
	var tabs []string
	for i, n := range names {
		label := n
		if m.w >= 60 {
			label = fmt.Sprintf("%d %s", i+1, n)
		}
		st := sTab
		if i == m.tab {
			st = sTabOn
		}
		tabs = append(tabs, m.btn(fmt.Sprintf("tab:%d", i), label, st))
	}
	title := sTitle.Render("♾ Infinito")
	quit := m.btn("quit", "✕", sBtn)
	header := row(m.w-lipgloss.Width(quit)-1, append([]string{title}, tabs...)...)
	hl := strings.Split(header, "\n")
	hl[0] = hl[0] + strings.Repeat(" ", max(1, m.w-lipgloss.Width(hl[0])-lipgloss.Width(quit))) + quit
	header = strings.Join(hl, "\n")

	bodyH := max(1, m.h-lipgloss.Height(header)-2)
	var body string
	if m.logOpen {
		body = m.viewLog()
	} else {
		switch m.tab {
		case tabHome:
			body = m.viewHome()
		case tabChat:
			body = m.viewChat(bodyH)
		case tabFiles:
			body = m.viewFiles(bodyH)
		case tabModels:
			body = m.viewModels(bodyH)
		case tabGuides:
			body = m.viewGuides(bodyH)
		case tabSettings:
			if m.sform == nil {
				m.openSettings()
			}
			body = m.formView(m.sform, bodyH)
		}
	}
	body = lipgloss.NewStyle().Height(bodyH).MaxHeight(bodyH).MaxWidth(m.w).Render(body)
	return zone.Scan(header + "\n" + body + "\n" + m.footer())
}

func (m *model) footer() string {
	if m.flash != "" {
		return lipgloss.NewStyle().MaxWidth(m.w).Render(sWarn.Render(" " + m.flash))
	}
	var h string
	switch m.tab {
	case tabHome:
		h = "Toca un botón · r comprobar"
	case tabChat:
		h = "Enter enviar · Alt+Enter línea · Esc parar · /ayuda"
	case tabFiles:
		h = "↑/↓ elegir · Enter abrir · d borrar"
	case tabModels:
		h = "Toca un modelo para usarlo en el chat"
	case tabGuides:
		h = "Rueda o PgUp/PgDn para leer"
	case tabSettings:
		h = "↑/↓ campo · Enter guardar · Esc descartar"
	}
	nav := "Tab pestañas · Ctrl+C salir"
	if !m.inputActive() {
		nav = "1-6 pestañas · q salir"
	}
	return lipgloss.NewStyle().MaxWidth(m.w).Render(sHelp.Render(" " + h + " · " + nav))
}

// ---------- Registro de FreeLLMAPI ----------

func (m *model) refreshLog() {
	lines := m.apps.Log("freellmapi", 400)
	atBottom := m.logVP.AtBottom()
	m.logVP.SetContent(lipgloss.NewStyle().Width(m.logVP.Width).Render(strings.Join(lines, "\n")))
	if atBottom {
		m.logVP.GotoBottom()
	}
}

func (m *model) viewLog() string {
	head := sTitle.Render("Registro de FreeLLMAPI") + "  " + m.btn("log:close", "Cerrar", sBtnPri)
	return head + "\n" + m.logVP.View()
}

// ---------- Utilidades ----------

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "••••"
	}
	return k[:4] + "…" + k[len(k)-4:]
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
