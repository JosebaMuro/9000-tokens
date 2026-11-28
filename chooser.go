package main

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// Pantalla inicial: elegir entre la interfaz gráfica y la de terminal.

type chooser struct {
	w, h     int
	cursor   int // 0 GUI, 1 TUI
	remember bool
	choice   string
}

func chooseMode(cfg *Config) string {
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return "gui"
	}
	setupTerminal()
	m := &chooser{}
	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		return "tui"
	}
	if m.choice != "" && m.remember {
		cfg.mu.Lock()
		cfg.StartMode = m.choice
		cfg.mu.Unlock()
		_ = cfg.Save()
	}
	return m.choice
}

func (m *chooser) Init() tea.Cmd { return tea.SetWindowTitle("Infinito") }

func (m *chooser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "left", "up", "h", "k", "shift+tab":
			m.cursor = 0
		case "right", "down", "l", "j", "tab":
			m.cursor = 1
		case "1", "g":
			m.choice = "gui"
			return m, tea.Quit
		case "2", "t":
			m.choice = "tui"
			return m, tea.Quit
		case "r", " ":
			m.remember = !m.remember
		case "enter":
			m.choice = []string{"gui", "tui"}[m.cursor]
			return m, tea.Quit
		}
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
			if msg.Action == tea.MouseActionMotion {
				switch {
				case zone.Get("c:gui").InBounds(msg):
					m.cursor = 0
				case zone.Get("c:tui").InBounds(msg):
					m.cursor = 1
				}
			}
			return m, nil
		}
		switch {
		case zone.Get("c:gui").InBounds(msg):
			m.choice = "gui"
			return m, tea.Quit
		case zone.Get("c:tui").InBounds(msg):
			m.choice = "tui"
			return m, tea.Quit
		case zone.Get("c:rem").InBounds(msg):
			m.remember = !m.remember
		case zone.Get("c:quit").InBounds(msg):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *chooser) View() string {
	if m.w == 0 {
		return ""
	}
	cardW := min(36, max(20, (m.w-6)/2))
	vertical := m.w < 2*cardW+6
	if vertical {
		cardW = min(m.w-4, 50)
	}
	card := func(id string, i int, icon, title, desc string) string {
		st := sCard
		if m.cursor == i {
			st = sCardOn
		}
		body := sTitle.Render(icon+"  "+title) + "\n\n" + lipgloss.NewStyle().Width(cardW-4).Render(desc)
		return zone.Mark(id, st.Width(cardW).Padding(1, 2).Render(body))
	}
	gui := card("c:gui", 0, "▣", "1 · Aplicación (GUI)", "Ventana visual con chat, imágenes, vídeo y voz. Se abre en tu navegador.")
	tui := card("c:tui", 1, "❯_", "2 · Terminal (TUI)", "Todo dentro de esta terminal. Funciona con teclado, ratón y táctil.")
	var cards string
	if vertical {
		cards = lipgloss.JoinVertical(lipgloss.Left, gui, tui)
	} else {
		cards = lipgloss.JoinHorizontal(lipgloss.Top, gui, "  ", tui)
	}
	box := "☐"
	if m.remember {
		box = "☑"
	}
	rem := zone.Mark("c:rem", sDim.Render(box+" Recordar mi elección (se cambia en Ajustes)"))
	quit := zone.Mark("c:quit", sBtn.Render("Salir"))
	out := sTitle.Render("♾ Infinito "+version) + "\n" + sDim.Render("¿Cómo quieres usarlo?") + "\n\n" +
		cards + "\n\n" + rem + "   " + quit + "\n\n" +
		sHelp.Render(strings.Join([]string{"Toca una opción", "← → y Enter", "1/2", "r recordar", "q salir"}, " · "))
	return zone.Scan(lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, out))
}
