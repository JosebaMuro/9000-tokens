package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

var tabNames = []string{"Inicio", "Chat", "Modelos", "Proveedores", "Claude Code", "Guías", "Ajustes"}
var tabShort = []string{"Inicio", "Chat", "Modelos", "Prov.", "Claude", "Guías", "Ajustes"}

const (
	tabHome = iota
	tabChat
	tabModels
	tabProviders
	tabClaude
	tabGuides
	tabSettings
)

// ---------- Mensajes ----------

type tickMsg time.Time
type backendStatusMsg struct {
	id  string
	n   int
	err error
}
type modelsMsg struct {
	backend string
	list    []string
	err     error
}
type chatEvMsg struct {
	ev chatEvent
	ch <-chan chatEvent
}
type provTestMsg struct {
	id, out string
	err     error
}
type provModelsMsg struct {
	id   string
	list []string
	err  error
}
type execDoneMsg struct{ err error }
type installDoneMsg struct{ err error }

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

type bstatus struct {
	checked bool
	n       int
	err     error
}

type storedMsg struct {
	chatMsg
	Via string `json:"via,omitempty"`
}

type model struct {
	cfg  *Config
	gw   *Gateway
	apps *AppManager
	w, h int
	tab  int

	zones     []string
	flash     string
	flashTime time.Time

	// Inicio
	status     map[string]bstatus
	installing bool
	installPr  *progress
	installErr string
	logView    string // id de la app cuyo registro se muestra
	logVP      viewport.Model

	// Chat
	vp        viewport.Model
	ta        textarea.Model
	history   []storedMsg
	partial   string
	streaming bool
	cancel    context.CancelFunc
	chatRoute string
	chatErr   string

	// Modelos
	modelsBackend int
	models        map[string][]string
	modelsErr     map[string]string
	modelsLoading bool
	mlist         *pickList

	// Proveedores
	pcursor int
	pform   *form
	pformID string
	ppick   *pickList
	ppickID string
	pmsg    map[string]string

	// Claude
	ccursor int

	// Guías
	guides  []guide
	gcursor int
	gvp     viewport.Model

	// Ajustes
	sform *form
}

func newModel(cfg *Config, gw *Gateway, apps *AppManager) *model {
	ta := textarea.New()
	ta.Placeholder = "Escribe tu mensaje… (Enter envía · Alt+Enter salto de línea)"
	ta.ShowLineNumbers = false
	ta.Prompt = "┃ "
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.KeyMap.InsertNewline.SetKeys("alt+enter", "ctrl+j")
	m := &model{
		cfg: cfg, gw: gw, apps: apps,
		status:    map[string]bstatus{},
		ta:        ta,
		vp:        viewport.New(80, 20),
		gvp:       viewport.New(80, 20),
		logVP:     viewport.New(80, 20),
		models:    map[string][]string{},
		modelsErr: map[string]string{},
		pmsg:      map[string]string{},
		guides:    loadGuides(),
	}
	m.loadHistory()
	for i, b := range cfg.backends() {
		if b.ID == cfg.ChatBackend {
			m.modelsBackend = i
		}
	}
	return m
}

func historyPath() string { return filepath.Join(configDir(), "chat.json") }

func (m *model) loadHistory() {
	b, err := os.ReadFile(historyPath())
	if err == nil {
		_ = json.Unmarshal(b, &m.history)
	}
}

func (m *model) saveHistory() {
	b, _ := json.MarshalIndent(m.history, "", " ")
	_ = os.MkdirAll(configDir(), 0o700)
	_ = os.WriteFile(historyPath(), b, 0o600)
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.checkBackends(), tick(), tea.SetWindowTitle("Infinito"))
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) setFlash(s string) {
	m.flash = s
	m.flashTime = time.Now()
}

func (m *model) checkBackends() tea.Cmd {
	var cmds []tea.Cmd
	for _, b := range m.cfg.backends() {
		b := b
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			l, err := listModels(ctx, v1(b.URL), b.APIKey)
			return backendStatusMsg{id: b.ID, n: len(l), err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m *model) loadModels(b Backend) tea.Cmd {
	m.modelsLoading = true
	return func() tea.Msg {
		l, err := listModels(context.Background(), v1(b.URL), b.APIKey)
		return modelsMsg{backend: b.ID, list: l, err: err}
	}
}

func (m *model) inputActive() bool {
	switch {
	case m.tab == tabChat:
		return true
	case m.tab == tabProviders && (m.pform != nil || (m.ppick != nil && m.ppick.filtOn)):
		return true
	case m.tab == tabSettings:
		return true
	case m.tab == tabModels && m.mlist != nil && m.mlist.filtOn:
		return true
	}
	return false
}

func (m *model) switchTab(t int) tea.Cmd {
	m.tab = (t + len(tabNames)) % len(tabNames)
	var cmds []tea.Cmd
	if m.tab == tabChat {
		cmds = append(cmds, m.ta.Focus())
		m.renderChat()
	} else {
		m.ta.Blur()
	}
	switch m.tab {
	case tabModels:
		b := m.cfg.backends()[m.modelsBackend]
		if l, ok := m.models[b.ID]; ok {
			if m.mlist == nil {
				m.mlist = newPickList("mlist", l, false, nil)
			}
		} else {
			cmds = append(cmds, m.loadModels(b))
		}
	case tabSettings:
		m.openSettings()
	case tabGuides:
		m.refreshGuide()
	case tabHome:
		cmds = append(cmds, m.checkBackends())
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
		if m.logView != "" {
			m.refreshLog()
		}
		return m, tick()

	case backendStatusMsg:
		m.status[msg.id] = bstatus{checked: true, n: msg.n, err: msg.err}
		return m, nil

	case installDoneMsg:
		m.installing = false
		if msg.err != nil {
			m.installErr = msg.err.Error()
			m.setFlash("La instalación falló: " + msg.err.Error())
		} else {
			m.installErr = ""
			m.setFlash("Instalado. Pulsa Arrancar en la pasarela que quieras usar.")
		}
		return m, nil

	case modelsMsg:
		m.modelsLoading = false
		if msg.err != nil {
			m.modelsErr[msg.backend] = msg.err.Error()
		} else {
			delete(m.modelsErr, msg.backend)
			m.models[msg.backend] = msg.list
		}
		if m.cfg.backends()[m.modelsBackend].ID == msg.backend {
			m.mlist = newPickList("mlist", m.models[msg.backend], false, nil)
		}
		return m, nil

	case chatEvMsg:
		return m, m.onChatEvent(msg)

	case provTestMsg:
		if msg.err != nil {
			m.pmsg[msg.id] = sBad.Render("✗ " + short(msg.err.Error(), 200))
		} else {
			m.pmsg[msg.id] = sOK.Render("✓ responde: " + msg.out)
		}
		return m, nil

	case provModelsMsg:
		if msg.err != nil {
			m.pmsg[msg.id] = sBad.Render("✗ " + short(msg.err.Error(), 200))
			return m, nil
		}
		if p := m.providerByID(msg.id); p != nil {
			m.ppick = newPickList("ppick", msg.list, true, p.Models)
			m.ppickID = msg.id
			m.pmsg[msg.id] = sDim.Render(fmt.Sprintf("%d modelos disponibles", len(msg.list)))
		}
		return m, nil

	case execDoneMsg:
		if msg.err != nil {
			m.setFlash("Terminó con error: " + msg.err.Error())
		} else {
			m.setFlash("Has vuelto de Claude Code")
		}
		return m, m.checkBackends()

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
	case m.logView != "":
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
	case m.tab == tabProviders && m.ppick != nil:
		m.ppick.scroll(d)
	case m.tab == tabProviders && m.pform == nil:
		m.pcursor = min(len(m.cfg.Providers)-1, max(0, m.pcursor+d/3))
	case m.tab == tabClaude:
		m.ccursor = min(len(m.claudeOpts())-1, max(0, m.ccursor+d/3))
	case m.tab == tabSettings && m.sform != nil:
		m.sform.move(d / 3)
	}
	return nil
}

// act ejecuta la acción de un botón o zona pulsada.
func (m *model) act(id string) tea.Cmd {
	parts := strings.Split(id, ":")
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
		return m.homeAction(arg(1), arg(2))
	case "chat":
		return m.chatAction(arg(1), arg(2))
	case "mback":
		return m.setModelsBackend(num(1))
	case "mlist":
		if m.mlist == nil {
			return nil
		}
		if arg(1) == "filter" {
			m.mlist.filtOn = true
			return m.mlist.filter.Focus()
		}
		m.mlist.cursor = num(2)
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
	case "prow":
		m.pcursor = num(1)
		if arg(2) == "toggle" {
			return m.keyProviders(keyRune(' '))
		}
	case "pact":
		return m.providerAction(arg(1))
	case "ppick":
		if m.ppick == nil {
			return nil
		}
		switch arg(1) {
		case "filter":
			m.ppick.filtOn = true
			return m.ppick.filter.Focus()
		case "row":
			m.ppick.cursor = num(2)
			return m.keyProviders(keyRune(' '))
		case "save":
			return m.keyProviders(tea.KeyMsg{Type: tea.KeyEnter})
		case "cancel":
			return m.keyProviders(tea.KeyMsg{Type: tea.KeyEsc})
		}
	case "pform", "sform":
		f := m.pform
		if parts[0] == "sform" {
			f = m.sform
		}
		if f == nil {
			return nil
		}
		switch arg(1) {
		case "field":
			f.setFocus(num(2))
		case "save":
			return m.formSubmit(parts[0])
		case "cancel":
			return m.formCancel(parts[0])
		case "reveal":
			f.toggleSecret()
		}
	case "claude":
		m.ccursor = num(1)
		return m.keyClaude("enter")
	case "guide":
		m.gcursor = num(1)
		m.refreshGuide()
	case "log":
		if arg(1) == "close" {
			m.logView = ""
		}
	}
	return nil
}

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func (m *model) quit() tea.Cmd {
	if m.streaming && m.cancel != nil {
		m.cancel()
	}
	return tea.Quit
}

func (m *model) onKey(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		return m.quit()
	}
	if m.logView != "" {
		switch s {
		case "esc", "q", "enter":
			m.logView = ""
			return nil
		}
		var c tea.Cmd
		m.logVP, c = m.logVP.Update(k)
		return c
	}
	modal := (m.tab == tabProviders && (m.pform != nil || m.ppick != nil)) || m.tab == tabSettings
	if !modal {
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
	case tabModels:
		return m.keyModels(k)
	case tabProviders:
		return m.keyProviders(k)
	case tabClaude:
		return m.keyClaude(s)
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
	if m.logView != "" {
		body = m.viewLog()
	} else {
		switch m.tab {
		case tabHome:
			body = m.viewHome(bodyH)
		case tabChat:
			body = m.viewChat()
		case tabModels:
			body = m.viewModels(bodyH)
		case tabProviders:
			body = m.viewProviders(bodyH)
		case tabClaude:
			body = m.viewClaude()
		case tabGuides:
			body = m.viewGuides()
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
		h = "Toca un botón o usa r comprobar · i instalar todo"
	case tabChat:
		h = "Enter enviar · Alt+Enter línea · Esc parar"
	case tabModels:
		h = "Toca un modelo para usarlo en el chat"
	case tabProviders:
		h = "Toca una fila y luego un botón"
	case tabClaude:
		h = "Toca una opción para lanzarla"
	case tabGuides:
		h = "Rueda o PgUp/PgDn para leer"
	case tabSettings:
		h = "↑/↓ campo · Enter guardar · Esc descartar"
	}
	nav := "Tab pestañas · Ctrl+C salir"
	if !m.inputActive() {
		nav = "1-7 pestañas · q salir"
	}
	return lipgloss.NewStyle().MaxWidth(m.w).Render(sHelp.Render(" " + h + " · " + nav))
}

// ---------- Registro de una app ----------

func (m *model) refreshLog() {
	lines := m.apps.Log(m.logView, 400)
	atBottom := m.logVP.AtBottom()
	m.logVP.SetContent(lipgloss.NewStyle().Width(m.logVP.Width).Render(strings.Join(lines, "\n")))
	if atBottom {
		m.logVP.GotoBottom()
	}
}

func (m *model) viewLog() string {
	head := sTitle.Render("Registro de "+appName(m.logView)) + "  " + m.btn("log:close", "Cerrar", sBtnPri)
	return head + "\n" + m.logVP.View()
}

// ---------- Utilidades ----------

func splitList(s string) []string {
	var out []string
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

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
