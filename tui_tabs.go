package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ======================= Inicio =======================

func (m *model) startInstall(only map[string]bool) tea.Cmd {
	if m.installing {
		return nil
	}
	if !hasPack() {
		m.setFlash("Este binario es la versión ligera: descarga la versión completa del release para instalar sin internet")
		return nil
	}
	m.apps.StopAll()
	m.installing = true
	m.installErr = ""
	m.installPr = &progress{}
	pr := m.installPr
	return func() tea.Msg { return installDoneMsg{installPack(pr, only)} }
}

func (m *model) homeAction(what, id string) tea.Cmd {
	switch what {
	case "install":
		if id == "all" {
			return m.startInstall(nil)
		}
		return m.startInstall(map[string]bool{"runtime": !nodeInstalled(), id: true})
	case "start":
		if id == "infinito" {
			if err := m.gw.Start(); err != nil {
				m.setFlash("No arranca: " + err.Error())
			}
			return m.checkBackends()
		}
		if err := m.apps.Start(id); err != nil {
			m.setFlash(err.Error())
			return nil
		}
		m.setFlash(appName(id) + " arrancando… tarda unos segundos")
		return tea.Sequence(tea.Tick(6*time.Second, func(time.Time) tea.Msg { return nil }), m.checkBackends())
	case "stop":
		if id == "infinito" {
			m.gw.Stop()
		} else {
			m.apps.Stop(id)
		}
		m.setFlash(appName(id) + " detenido")
		return tea.Sequence(tea.Tick(time.Second, func(time.Time) tea.Msg { return nil }), m.checkBackends())
	case "use":
		m.cfg.ChatBackend = id
		m.cfg.ChatModel = "auto"
		_ = m.cfg.Save()
		for i, b := range m.cfg.backends() {
			if b.ID == id {
				m.modelsBackend = i
			}
		}
		m.setFlash("Ahora el chat usa " + appName(id) + ". La conversación se mantiene.")
		return m.switchTab(tabChat)
	case "web":
		u := m.cfg.backendByID(id).Dashboard
		if u == "" {
			u = m.cfg.backendByID(id).URL
		}
		if err := openURL(u); err != nil {
			m.setFlash(err.Error())
		} else {
			m.setFlash("Abriendo " + u)
		}
	case "log":
		m.logView = id
		m.refreshLog()
		m.logVP.GotoBottom()
	case "providers":
		return m.switchTab(tabProviders)
	case "refresh":
		m.status = map[string]bstatus{}
		return m.checkBackends()
	}
	return nil
}

func (m *model) keyHome(s string) tea.Cmd {
	switch s {
	case "r":
		return m.homeAction("refresh", "")
	case "i":
		return m.homeAction("install", "all")
	case "g":
		if on, _ := m.gw.Running(); on {
			return m.homeAction("stop", "infinito")
		}
		return m.homeAction("start", "infinito")
	}
	return nil
}

func (m *model) running(id string) bool {
	if id == "infinito" {
		on, _ := m.gw.Running()
		return on
	}
	return m.apps.Running(id)
}

func (m *model) viewHome(height int) string {
	var sb strings.Builder
	cw := min(m.w, 100)
	inner := cw - 4

	if m.installing && m.installPr != nil {
		done, total := m.installPr.done.Load(), m.installPr.total.Load()
		pct := 0.0
		if total > 0 {
			pct = float64(done) / float64(total)
		}
		barW := max(10, inner-8)
		fill := int(pct * float64(barW))
		bar := sOK.Render(strings.Repeat("█", fill)) + sDim.Render(strings.Repeat("░", barW-fill))
		f, _ := m.installPr.file.Load().(string)
		sb.WriteString(sCardOn.Width(cw - 2).Render(sTitle.Render("Instalando sin internet…") + "\n" + bar + fmt.Sprintf(" %3.0f%%", pct*100) + "\n" + sDim.Render(truncate(f, inner))))
		sb.WriteString("\n")
	} else if hasPack() && (!m.apps.Installed("omniroute") || !m.apps.Installed("freellmapi")) {
		msg := sTitle.Render("Paquete offline incluido") + "\n" +
			"Este binario trae Node.js, OmniRoute y FreeLLMAPI. Instálalos una vez y funcionan sin internet.\n" +
			m.btn("home:install:all", "Instalar todo", sBtnPri)
		if m.installErr != "" {
			msg += "\n" + sBad.Render("Último error: "+m.installErr)
		}
		sb.WriteString(sCardOn.Width(cw-2).Render(msg) + "\n")
	}

	for _, b := range m.cfg.backends() {
		id := b.ID
		st, checked := m.status[id]
		active := m.cfg.ChatBackend == id
		var state string
		switch {
		case checked && st.err == nil && id == "infinito" && m.running(id):
			state = sOK.Render("● en marcha")
		case checked && st.err == nil && id != "infinito":
			state = sOK.Render(fmt.Sprintf("● en marcha · %d modelos", st.n))
		case m.running(id):
			state = sWarn.Render("● arrancando…")
		case checked && st.err != nil && strings.Contains(st.err.Error(), "HTTP 401"):
			state = sWarn.Render("● en marcha · falta la clave (Ajustes)")
		default:
			state = sDim.Render("○ parado")
			if id != "infinito" {
				if err := m.apps.LastError(id); err != nil {
					state = sBad.Render("● " + short(err.Error(), 50))
				}
			}
		}
		name := sBold.Render(appName(id))
		if active {
			name += " " + sSel.Render("★ en uso")
		}
		desc := ""
		switch id {
		case "infinito":
			n := len(m.gw.chain())
			desc = fmt.Sprintf("Incluida en este programa · %d rutas en la cadena", n)
			if n == 0 {
				desc = sWarn.Render("Sin claves todavía: pulsa Proveedores")
			}
		case "omniroute":
			desc = "150+ proveedores gratis · panel web en " + b.Dashboard
		case "freellmapi":
			desc = "87 proveedores gratis · panel web en " + b.Dashboard
		}
		var btns []string
		installed := id == "infinito" || m.apps.Installed(id)
		ext := false // en marcha fuera de Infinito
		if id != "infinito" && checked && st.err == nil && !m.apps.Running(id) {
			ext = true
		}
		switch {
		case !installed && !ext:
			if hasPack() {
				btns = append(btns, m.btn("home:install:"+id, "Instalar", sBtnPri))
			} else {
				btns = append(btns, sDim.Render("No instalado (guía en Guías)"))
			}
		case m.running(id):
			btns = append(btns, m.btn("home:stop:"+id, "Parar", sBtnBad))
		case !ext:
			btns = append(btns, m.btn("home:start:"+id, "Arrancar", sBtnPri))
		}
		if !active {
			btns = append(btns, m.btn("home:use:"+id, "Usar en el chat", sBtn))
		}
		if id == "infinito" {
			btns = append(btns, m.btn("home:providers:", "Proveedores", sBtn))
		} else {
			btns = append(btns, m.btn("home:web:"+id, "Panel web", sBtn))
			if m.apps.Installed(id) {
				btns = append(btns, m.btn("home:log:"+id, "Registro", sBtn))
			}
		}
		content := name + "\n" + state + "\n" + sDim.Render(truncate(desc, inner*2)) + "\n" + row(inner, btns...)
		cs := sCard
		if active {
			cs = sCardOn
		}
		sb.WriteString(cs.Width(cw-2).Render(content) + "\n")
	}
	sb.WriteString(row(m.w, m.btn("home:refresh:", "Comprobar de nuevo", sBtn), m.btn("tab:4", "Lanzar Claude Code", sBtn), m.btn("tab:5", "Guías", sBtn)))

	// Actividad de la pasarela integrada, si cabe.
	used := lipgloss.Height(sb.String())
	if rest := height - used - 2; rest >= 3 {
		logs := m.gw.Logs(rest - 1)
		if len(logs) > 0 {
			sb.WriteString("\n" + sTitle.Render("Actividad") + "\n")
			for _, l := range logs {
				sb.WriteString(" " + sDim.Render(truncate(l, m.w-2)) + "\n")
			}
		}
	}
	return sb.String()
}

// ======================= Chat =======================

func (m *model) chatBackend() Backend { return m.cfg.backendByID(m.cfg.ChatBackend) }

func (m *model) chatAction(what, arg string) tea.Cmd {
	switch what {
	case "send":
		return m.keyChat(tea.KeyMsg{Type: tea.KeyEnter})
	case "stop":
		if m.streaming && m.cancel != nil {
			m.cancel()
		}
	case "new":
		return m.keyChat(tea.KeyMsg{Type: tea.KeyCtrlN})
	case "backend":
		m.cfg.ChatBackend = arg
		m.cfg.ChatModel = "auto"
		_ = m.cfg.Save()
		m.setFlash("Ahora respondes con " + appName(arg) + " · la conversación sigue")
		return m.ta.Focus()
	case "model":
		for i, b := range m.cfg.backends() {
			if b.ID == m.cfg.ChatBackend {
				m.modelsBackend = i
			}
		}
		m.mlist = nil
		return m.switchTab(tabModels)
	case "focus":
		return m.ta.Focus()
	}
	return nil
}

func (m *model) keyChat(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "enter":
		if m.streaming {
			return nil
		}
		text := strings.TrimSpace(m.ta.Value())
		if text == "" {
			return nil
		}
		m.ta.Reset()
		if strings.HasPrefix(text, "/") {
			return m.chatCommand(text)
		}
		m.history = append(m.history, storedMsg{chatMsg: chatMsg{Role: "user", Content: text}})
		m.saveHistory()
		return m.sendChat()
	case "esc":
		if m.streaming && m.cancel != nil {
			m.cancel()
		}
		return nil
	case "ctrl+n":
		if m.streaming && m.cancel != nil {
			m.cancel()
		}
		m.history, m.partial, m.chatErr, m.chatRoute = nil, "", "", ""
		m.saveHistory()
		m.renderChat()
		return nil
	case "ctrl+b":
		bs := m.cfg.backends()
		for i, b := range bs {
			if b.ID == m.cfg.ChatBackend {
				return m.chatAction("backend", bs[(i+1)%len(bs)].ID)
			}
		}
	case "ctrl+o":
		return m.chatAction("model", "")
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		var c tea.Cmd
		m.vp, c = m.vp.Update(k)
		return c
	case "up":
		if m.ta.Line() == 0 {
			m.vp.LineUp(1)
			return nil
		}
	case "down":
		if m.ta.Line() == m.ta.LineCount()-1 {
			m.vp.LineDown(1)
			return nil
		}
	}
	var c tea.Cmd
	m.ta, c = m.ta.Update(k)
	return c
}

func (m *model) chatCommand(cmd string) tea.Cmd {
	f := strings.Fields(cmd)
	switch f[0] {
	case "/modelo", "/model":
		if len(f) > 1 {
			m.cfg.ChatModel = f[1]
			_ = m.cfg.Save()
			m.setFlash("Modelo: " + f[1])
		}
	case "/sistema", "/system":
		m.cfg.ChatSystem = strings.TrimSpace(strings.TrimPrefix(cmd, f[0]))
		_ = m.cfg.Save()
		m.setFlash("Prompt de sistema actualizado")
	case "/nuevo", "/new", "/clear":
		return m.keyChat(tea.KeyMsg{Type: tea.KeyCtrlN})
	case "/pasarela", "/repo":
		if len(f) > 1 {
			for _, b := range m.cfg.backends() {
				if strings.EqualFold(b.ID, f[1]) || strings.EqualFold(appName(b.ID), f[1]) {
					return m.chatAction("backend", b.ID)
				}
			}
		}
		m.setFlash("Usa: /pasarela infinito | omniroute | freellmapi")
	default:
		m.setFlash("Comandos: /pasarela NOMBRE · /modelo NOMBRE · /sistema TEXTO · /nuevo")
	}
	return nil
}

func (m *model) sendChat() tea.Cmd {
	b := m.chatBackend()
	msgs := []chatMsg{}
	if m.cfg.ChatSystem != "" {
		msgs = append(msgs, chatMsg{Role: "system", Content: m.cfg.ChatSystem})
	}
	for _, h := range m.history {
		msgs = append(msgs, h.chatMsg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true
	m.partial, m.chatErr, m.chatRoute = "", "", ""
	ch := chatStream(ctx, v1(b.URL), b.APIKey, m.cfg.ChatModel, msgs)
	m.renderChat()
	return waitChat(ch)
}

func waitChat(ch <-chan chatEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return chatEvMsg{ev: chatEvent{Done: true}}
		}
		return chatEvMsg{ev: ev, ch: ch}
	}
}

func (m *model) onChatEvent(msg chatEvMsg) tea.Cmd {
	ev := msg.ev
	switch {
	case ev.Err != nil:
		if !strings.Contains(ev.Err.Error(), "context canceled") {
			m.chatErr = ev.Err.Error()
		}
		m.finishChat()
		return nil
	case ev.Route != "":
		m.chatRoute = ev.Route
	case ev.Delta != "":
		m.partial += ev.Delta
	case ev.Done:
		m.finishChat()
		return nil
	}
	m.renderChat()
	if msg.ch == nil {
		return nil
	}
	return waitChat(msg.ch)
}

func (m *model) finishChat() {
	if !m.streaming {
		return
	}
	m.streaming = false
	if m.partial != "" {
		via := appName(m.cfg.ChatBackend)
		if m.chatRoute != "" {
			via += " · " + m.chatRoute
		}
		m.history = append(m.history, storedMsg{chatMsg: chatMsg{Role: "assistant", Content: m.partial}, Via: via})
		m.saveHistory()
	}
	m.partial = ""
	m.renderChat()
}

func (m *model) renderChat() {
	w := max(20, m.vp.Width-1)
	wrap := lipgloss.NewStyle().Width(w)
	var sb strings.Builder
	if len(m.history) == 0 && !m.streaming {
		sb.WriteString(sDim.Render(wrap.Render("Escribe abajo y pulsa Enter (o el botón Enviar).\n\nArriba eliges con qué pasarela responde la IA. Puedes cambiar de pasarela cuando quieras: la conversación se mantiene y la siguiente respuesta la da la nueva.\n\nComandos: /pasarela NOMBRE · /modelo NOMBRE · /sistema TEXTO · /nuevo")))
	}
	for _, h := range m.history {
		if h.Role == "user" {
			sb.WriteString(sUser.Render("Tú") + "\n" + wrap.Render(h.Content) + "\n\n")
		} else {
			sb.WriteString(sBot.Render("IA") + sDim.Render(" · "+h.Via) + "\n" + wrap.Render(h.Content) + "\n\n")
		}
	}
	if m.streaming {
		via := appName(m.cfg.ChatBackend)
		if m.chatRoute != "" {
			via += " · " + m.chatRoute
		}
		sb.WriteString(sBot.Render("IA") + sDim.Render(" · "+via+" · escribiendo…") + "\n" + wrap.Render(m.partial+"▌") + "\n")
	}
	if m.chatErr != "" {
		sb.WriteString(sBad.Render(wrap.Render("Error: "+m.chatErr)) + "\n" +
			sDim.Render(wrap.Render("Prueba con otra pasarela arriba, o revisa Inicio y Ajustes.")) + "\n")
	}
	m.vp.SetContent(sb.String())
	m.vp.GotoBottom()
}

func (m *model) viewChat() string {
	var chips []string
	for _, b := range m.cfg.backends() {
		st := sBtn
		if b.ID == m.cfg.ChatBackend {
			st = sBtnPri
		}
		label := appName(b.ID)
		if b.ID == "infinito" && m.narrow() {
			label = "Integrada"
		}
		chips = append(chips, m.btn("chat:backend:"+b.ID, label, st))
	}
	chips = append(chips, m.btn("chat:model:", "Modelo: "+truncate(m.cfg.ChatModel, 24)+" ▾", sBtn))
	top := row(m.w, chips...)
	var actions []string
	if m.streaming {
		actions = append(actions, m.btn("chat:stop:", "Parar", sBtnBad))
	} else {
		actions = append(actions, m.btn("chat:send:", "Enviar", sBtnPri))
	}
	actions = append(actions, m.btn("chat:new:", "Nueva conversación", sBtn))
	bottom := row(m.w, actions...)
	// el área de mensajes ocupa lo que quede
	m.vp.Height = max(3, m.h-4-lipgloss.Height(top)-lipgloss.Height(bottom)-m.ta.Height()-1)
	return top + "\n" + m.vp.View() + "\n" + m.mark("chat:focus:", m.ta.View()) + "\n" + bottom
}

// ======================= Modelos =======================

func (m *model) setModelsBackend(i int) tea.Cmd {
	bs := m.cfg.backends()
	m.modelsBackend = (i + len(bs)) % len(bs)
	nb := bs[m.modelsBackend]
	if l, ok := m.models[nb.ID]; ok {
		m.mlist = newPickList("mlist", l, false, nil)
		return nil
	}
	m.mlist = nil
	return m.loadModels(nb)
}

func (m *model) keyModels(k tea.KeyMsg) tea.Cmd {
	bs := m.cfg.backends()
	b := bs[m.modelsBackend]
	if m.mlist != nil {
		if ok, c := m.mlist.handleKey(k, m.h-10); ok {
			return c
		}
	}
	switch k.String() {
	case "left", "h":
		return m.setModelsBackend(m.modelsBackend - 1)
	case "right", "l":
		return m.setModelsBackend(m.modelsBackend + 1)
	case "r":
		delete(m.models, b.ID)
		m.mlist = nil
		return m.loadModels(b)
	case "enter":
		if m.mlist == nil || m.mlist.current() == "" {
			return nil
		}
		m.cfg.ChatBackend = b.ID
		m.cfg.ChatModel = m.mlist.current()
		_ = m.cfg.Save()
		m.setFlash("Chat: " + appName(b.ID) + " · " + m.cfg.ChatModel)
		return m.switchTab(tabChat)
	case "c":
		if m.mlist != nil && m.mlist.current() != "" {
			copyClipboard(m.mlist.current())
			m.setFlash("Copiado: " + m.mlist.current())
		}
	}
	return nil
}

func (m *model) viewModels(height int) string {
	bs := m.cfg.backends()
	var chips []string
	for i, b := range bs {
		st := sBtn
		if i == m.modelsBackend {
			st = sBtnPri
		}
		chips = append(chips, m.btn(fmt.Sprintf("mback:%d", i), appName(b.ID), st))
	}
	top := row(m.w, chips...)
	acts := row(m.w, m.btn("mact:filter", "Buscar", sBtn), m.btn("mact:reload", "Recargar", sBtn), m.btn("mact:copy", "Copiar nombre", sBtn))
	b := bs[m.modelsBackend]
	out := top + "\n" + acts + "\n"
	listH := height - lipgloss.Height(top) - lipgloss.Height(acts) - 2
	switch {
	case m.modelsLoading && m.mlist == nil:
		out += sDim.Render("Cargando modelos de " + v1(b.URL) + "…")
	case m.modelsErr[b.ID] != "":
		out += sBad.Render(lipgloss.NewStyle().Width(m.w).Render("No pude leer los modelos: "+m.modelsErr[b.ID])) + "\n\n" +
			sDim.Render(lipgloss.NewStyle().Width(m.w).Render("¿Está en marcha? Mira Inicio. ¿Falta la clave? Ponla en Ajustes."))
	case m.mlist != nil:
		out += m.pickView(m.mlist, listH)
	}
	return out
}

// ======================= Proveedores =======================

func (m *model) providerByID(id string) *Provider {
	for i := range m.cfg.Providers {
		if m.cfg.Providers[i].ID == id {
			return &m.cfg.Providers[i]
		}
	}
	return nil
}

func (m *model) providerAction(what string) tea.Cmd {
	switch what {
	case "edit":
		return m.keyProviders(keyRune('e'))
	case "toggle":
		return m.keyProviders(keyRune(' '))
	case "test":
		return m.keyProviders(keyRune('t'))
	case "fetch":
		return m.keyProviders(keyRune('f'))
	case "up":
		return m.keyProviders(keyRune('+'))
	case "down":
		return m.keyProviders(keyRune('-'))
	case "web":
		return m.keyProviders(keyRune('w'))
	case "add":
		return m.keyProviders(keyRune('a'))
	case "del":
		return m.keyProviders(tea.KeyMsg{Type: tea.KeyDelete})
	}
	return nil
}

func (m *model) formSubmit(which string) tea.Cmd {
	if which == "sform" {
		return m.keySettings(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m.keyProviders(tea.KeyMsg{Type: tea.KeyEnter})
}

func (m *model) formCancel(which string) tea.Cmd {
	if which == "sform" {
		return m.keySettings(tea.KeyMsg{Type: tea.KeyEsc})
	}
	return m.keyProviders(tea.KeyMsg{Type: tea.KeyEsc})
}

func (m *model) keyProviders(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	ps := m.cfg.Providers
	if m.pform != nil {
		switch s {
		case "esc":
			m.pform = nil
			return nil
		case "up", "shift+tab":
			m.pform.move(-1)
			return nil
		case "down", "tab":
			m.pform.move(1)
			return nil
		case "ctrl+r":
			m.pform.toggleSecret()
			return nil
		case "enter":
			v := m.pform.values()
			m.cfg.mu.Lock()
			if p := m.providerByID(m.pformID); p != nil {
				p.APIKey = v[0]
				p.BaseURL = v[1]
				p.Models = splitList(v[2])
				if p.APIKey != "" || p.NoKey {
					p.Enabled = true
				}
				if strings.HasPrefix(p.ID, "custom") && len(v) > 3 && v[3] != "" {
					p.Name = v[3]
				}
			}
			m.cfg.mu.Unlock()
			_ = m.cfg.Save()
			m.pform = nil
			m.setFlash("Guardado. Pulsa Probar para comprobarlo.")
			return nil
		}
		return m.pform.update(k)
	}
	if m.ppick != nil {
		if ok, c := m.ppick.handleKey(k, m.h-12); ok {
			return c
		}
		switch s {
		case "esc":
			m.ppick = nil
		case "enter":
			m.cfg.mu.Lock()
			if p := m.providerByID(m.ppickID); p != nil {
				var out []string
				for _, x := range p.Models {
					if m.ppick.checked[x] {
						out = append(out, x)
					}
				}
				for _, x := range m.ppick.items {
					if m.ppick.checked[x] && !contains(out, x) {
						out = append(out, x)
					}
				}
				p.Models = out
			}
			m.cfg.mu.Unlock()
			_ = m.cfg.Save()
			m.ppick = nil
			m.setFlash("Modelos guardados")
		}
		return nil
	}
	if len(ps) == 0 {
		return nil
	}
	m.pcursor = min(max(0, m.pcursor), len(ps)-1)
	cur := ps[m.pcursor]
	switch s {
	case "up", "k":
		m.pcursor = max(0, m.pcursor-1)
	case "down", "j":
		m.pcursor = min(len(ps)-1, m.pcursor+1)
	case " ", "x":
		m.cfg.mu.Lock()
		m.cfg.Providers[m.pcursor].Enabled = !cur.Enabled
		m.cfg.mu.Unlock()
		_ = m.cfg.Save()
		if !cur.Enabled && cur.APIKey == "" && !cur.NoKey {
			m.setFlash("Activado, pero falta la clave: pulsa Editar")
		}
	case "+", "K", "shift+up":
		if m.pcursor > 0 {
			m.cfg.mu.Lock()
			ps[m.pcursor], ps[m.pcursor-1] = ps[m.pcursor-1], ps[m.pcursor]
			m.cfg.mu.Unlock()
			m.pcursor--
			_ = m.cfg.Save()
		}
	case "-", "J", "shift+down":
		if m.pcursor < len(ps)-1 {
			m.cfg.mu.Lock()
			ps[m.pcursor], ps[m.pcursor+1] = ps[m.pcursor+1], ps[m.pcursor]
			m.cfg.mu.Unlock()
			m.pcursor++
			_ = m.cfg.Save()
		}
	case "enter", "e":
		labels := []string{"Clave API (pégala aquí)", "URL base (…/v1)", "Modelos, en orden, separados por comas"}
		vals := []string{cur.APIKey, cur.BaseURL, strings.Join(cur.Models, ", ")}
		secret := []bool{true, false, false}
		if strings.HasPrefix(cur.ID, "custom") {
			labels = append(labels, "Nombre")
			vals = append(vals, cur.Name)
			secret = append(secret, false)
		}
		title := cur.Name
		if cur.KeyURL != "" {
			title += " · clave en " + cur.KeyURL
		}
		m.pform = newForm("pform", title, labels, vals, secret, m.w)
		m.pformID = cur.ID
		return textinput.Blink
	case "t":
		m.pmsg[cur.ID] = sDim.Render("probando…")
		p := cur
		return func() tea.Msg {
			out, err := testProvider(context.Background(), p)
			return provTestMsg{id: p.ID, out: out, err: err}
		}
	case "f":
		m.pmsg[cur.ID] = sDim.Render("descargando lista de modelos…")
		p := cur
		return func() tea.Msg {
			l, err := listModels(context.Background(), p.BaseURL, p.APIKey)
			return provModelsMsg{id: p.ID, list: l, err: err}
		}
	case "a":
		n := 1
		for m.providerByID(fmt.Sprintf("custom%d", n)) != nil {
			n++
		}
		m.cfg.mu.Lock()
		m.cfg.Providers = append(m.cfg.Providers, Provider{ID: fmt.Sprintf("custom%d", n), Name: fmt.Sprintf("Personalizado %d", n), BaseURL: "https://", Models: []string{}})
		m.cfg.mu.Unlock()
		m.pcursor = len(m.cfg.Providers) - 1
		_ = m.cfg.Save()
		return m.keyProviders(keyRune('e'))
	case "delete":
		if strings.HasPrefix(cur.ID, "custom") {
			m.cfg.mu.Lock()
			m.cfg.Providers = append(ps[:m.pcursor], ps[m.pcursor+1:]...)
			m.cfg.mu.Unlock()
			_ = m.cfg.Save()
			m.pcursor = max(0, m.pcursor-1)
		} else {
			m.setFlash("Solo se pueden borrar los proveedores personalizados")
		}
	case "w":
		if cur.KeyURL != "" {
			_ = openURL(cur.KeyURL)
			copyClipboard(cur.KeyURL)
			m.setFlash("Abriendo " + cur.KeyURL)
		}
	}
	return nil
}

func (m *model) viewProviders(height int) string {
	if m.pform != nil {
		return m.formView(m.pform, height)
	}
	if m.ppick != nil {
		p := m.providerByID(m.ppickID)
		head := sTitle.Render("Elige los modelos de "+p.Name) + "\n" +
			row(m.w, m.btn("ppick:save", "Guardar", sBtnPri), m.btn("ppick:cancel", "Cancelar", sBtn), m.btn("ppick:filter", "Buscar", sBtn)) + "\n" +
			sDim.Render("Toca un modelo para marcarlo o desmarcarlo")
		return head + "\n" + m.pickView(m.ppick, height-lipgloss.Height(head)-1)
	}
	ps := m.cfg.Providers
	m.pcursor = min(max(0, m.pcursor), max(0, len(ps)-1))
	var cur Provider
	if len(ps) > 0 {
		cur = ps[m.pcursor]
	}
	toggle := "Activar"
	if cur.Enabled {
		toggle = "Desactivar"
	}
	acts := []string{
		m.btn("pact:edit", "Poner clave", sBtnPri),
		m.btn("pact:toggle", toggle, sBtn),
		m.btn("pact:test", "Probar", sBtn),
		m.btn("pact:fetch", "Elegir modelos", sBtn),
		m.btn("pact:up", "▲ Subir", sBtn),
		m.btn("pact:down", "▼ Bajar", sBtn),
	}
	if cur.KeyURL != "" {
		acts = append(acts, m.btn("pact:web", "Web de la clave", sBtn))
	}
	acts = append(acts, m.btn("pact:add", "+ Añadir propio", sBtn))
	if strings.HasPrefix(cur.ID, "custom") {
		acts = append(acts, m.btn("pact:del", "Borrar", sBtnBad))
	}
	actions := row(m.w, acts...)
	intro := sDim.Render(lipgloss.NewStyle().Width(m.w).Render("Proveedores de la pasarela integrada. El orden es la cadena de respaldo: si el de arriba llega a su límite, salta al siguiente."))
	info := ""
	if len(ps) > 0 {
		info = sDim.Render("Modelos: ") + truncate(strings.Join(cur.Models, " → "), m.w*2-10)
		if msg := m.pmsg[cur.ID]; msg != "" {
			info += "\n" + msg
		}
		info = lipgloss.NewStyle().Width(m.w).Render(info)
	}
	rowsH := max(3, height-lipgloss.Height(intro)-lipgloss.Height(actions)-lipgloss.Height(info)-2)
	start := 0
	if m.pcursor >= rowsH {
		start = m.pcursor - rowsH + 1
	}
	var sb strings.Builder
	for i := start; i < len(ps) && i < start+rowsH; i++ {
		p := ps[i]
		on := sDim.Render("[ ]")
		if p.Enabled {
			on = sOK.Render("[x]")
		}
		key := sWarn.Render("sin clave")
		if p.NoKey {
			key = sDim.Render("no necesita")
		} else if p.APIKey != "" {
			key = sOK.Render("clave " + maskKey(p.APIKey))
		}
		nameW := 24
		if m.narrow() {
			nameW = max(10, m.w-30)
		}
		name := fmt.Sprintf("%-*s", nameW, truncate(p.Name, nameW))
		mark := "  "
		if i == m.pcursor {
			mark = sSel.Render("▸ ")
			name = sSel.Render(name)
		}
		line := mark + m.mark(fmt.Sprintf("prow:%d:toggle", i), on) + " " + m.mark(fmt.Sprintf("prow:%d", i), name+" "+key)
		if !m.narrow() && p.Note != "" {
			line += " " + sDim.Render(truncate(p.Note, m.w-60))
		}
		sb.WriteString(lipgloss.NewStyle().MaxWidth(m.w).Render(line) + "\n")
	}
	return intro + "\n" + sb.String() + info + "\n" + actions
}

// ======================= Claude Code =======================

type claudeOpt struct {
	title, desc string
}

func (m *model) claudeOpts() []claudeOpt {
	return []claudeOpt{
		{"Con la pasarela integrada", "Abre 'claude' conectado a " + m.cfg.gatewayURL() + " con el modelo auto."},
		{"Con OmniRoute", "Ejecuta 'omniroute run claude --model auto' con el OmniRoute instalado."},
		{"Con FreeLLMAPI", "Abre 'claude' conectado a " + m.cfg.FreeLLMAPI.URL + " con tu clave única."},
		{"Copiar las variables", "Para conectar Claude Code desde otra terminal a la pasarela integrada."},
	}
}

func (m *model) keyClaude(s string) tea.Cmd {
	opts := m.claudeOpts()
	switch s {
	case "up", "k":
		m.ccursor = max(0, m.ccursor-1)
	case "down", "j":
		m.ccursor = min(len(opts)-1, m.ccursor+1)
	case "enter":
		done := func(err error) tea.Msg { return execDoneMsg{err} }
		switch m.ccursor {
		case 0:
			if on, _ := m.gw.Running(); !on {
				if err := m.gw.Start(); err != nil {
					m.setFlash("La pasarela no arranca: " + err.Error())
					return nil
				}
			}
			if len(m.gw.chain()) == 0 {
				m.setFlash("Antes añade al menos un proveedor con clave (pestaña Proveedores)")
				return nil
			}
			cmd, err := claudeCmd(claudeEnv(m.cfg.gatewayURL(), m.cfg.GatewayKey, "auto"))
			if err != nil {
				m.setFlash(err.Error())
				return nil
			}
			return tea.ExecProcess(cmd, done)
		case 1:
			var cmd *exec.Cmd
			if m.apps.Installed("omniroute") {
				if which("claude") == "" {
					m.setFlash("No encuentro 'claude'. Instálalo con: npm install -g @anthropic-ai/claude-code")
					return nil
				}
				cmd = exec.Command(nodeBin(), appEntry("omniroute"), "run", "claude", "--model", "auto")
				cmd.Env = append(nodeEnv(), "DATA_DIR="+dataDir()+string(os.PathSeparator)+"data"+string(os.PathSeparator)+"omniroute")
			} else {
				var err error
				if cmd, err = omnirouteRunCmd("auto"); err != nil {
					m.setFlash("OmniRoute no está instalado: ve a Inicio y pulsa Instalar")
					return nil
				}
			}
			return tea.ExecProcess(cmd, done)
		case 2:
			if m.cfg.FreeLLMAPI.APIKey == "" {
				m.setFlash("Pon la clave única de FreeLLMAPI en Ajustes (la ves en su panel web, en Keys)")
				return nil
			}
			cmd, err := claudeCmd(claudeEnv(m.cfg.FreeLLMAPI.URL, m.cfg.FreeLLMAPI.APIKey, ""))
			if err != nil {
				m.setFlash(err.Error())
				return nil
			}
			return tea.ExecProcess(cmd, done)
		case 3:
			script := envScript(claudeEnv(m.cfg.gatewayURL(), m.cfg.GatewayKey, "auto"))
			copyClipboard(script)
			m.setFlash("Variables copiadas. También las muestra: infinito env")
		}
	}
	return nil
}

func (m *model) viewClaude() string {
	var sb strings.Builder
	cwd, _ := os.Getwd()
	sb.WriteString(sDim.Render(truncate("Se abrirá en la carpeta: "+cwd, m.w)) + "\n")
	if which("claude") == "" {
		sb.WriteString(sWarn.Render(lipgloss.NewStyle().Width(m.w).Render("No encuentro 'claude' en este equipo. Claude Code se instala aparte: npm install -g @anthropic-ai/claude-code")) + "\n")
	}
	sb.WriteString("\n")
	for i, o := range m.claudeOpts() {
		st := sCard
		if i == m.ccursor {
			st = sCardOn
		}
		card := st.Width(min(m.w, 90) - 2).Render(sBold.Render("▶ "+o.title) + "\n" + sDim.Render(o.desc))
		sb.WriteString(m.mark(fmt.Sprintf("claude:%d", i), card) + "\n")
	}
	sb.WriteString(sDim.Render("Al cerrar Claude Code vuelves aquí."))
	return sb.String()
}

// ======================= Guías =======================

func (m *model) refreshGuide() {
	if len(m.guides) == 0 {
		return
	}
	g := m.guides[m.gcursor]
	body := lipgloss.NewStyle().Width(max(20, m.gvp.Width-1)).Render(g.Body)
	m.gvp.SetContent(sTitle.Render(g.Title) + "\n\n" + body)
	m.gvp.GotoTop()
}

func (m *model) keyGuides(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "up", "k", "left":
		if m.gcursor > 0 {
			m.gcursor--
			m.refreshGuide()
		}
		return nil
	case "down", "j", "right":
		if m.gcursor < len(m.guides)-1 {
			m.gcursor++
			m.refreshGuide()
		}
		return nil
	}
	var c tea.Cmd
	m.gvp, c = m.gvp.Update(k)
	return c
}

func (m *model) viewGuides() string {
	if m.narrow() {
		var chips []string
		for i, g := range m.guides {
			st := sBtn
			if i == m.gcursor {
				st = sBtnPri
			}
			chips = append(chips, m.btn(fmt.Sprintf("guide:%d", i), truncate(g.Title, 22), st))
		}
		top := row(m.w, chips...)
		m.gvp.Height = max(3, m.h-5-lipgloss.Height(top))
		return top + "\n" + m.gvp.View()
	}
	var left strings.Builder
	for i, g := range m.guides {
		t := truncate(g.Title, guideListW()-3)
		if i == m.gcursor {
			left.WriteString(m.mark(fmt.Sprintf("guide:%d", i), sSel.Render("▸ "+t)) + "\n")
		} else {
			left.WriteString(m.mark(fmt.Sprintf("guide:%d", i), "  "+t) + "\n")
		}
	}
	l := lipgloss.NewStyle().Width(guideListW()).Render(left.String())
	return lipgloss.JoinHorizontal(lipgloss.Top, l, " ", m.gvp.View())
}

// ======================= Ajustes =======================

var settingsLabels = []string{
	"Puerto de la pasarela integrada", "Escuchar en (127.0.0.1 = solo este equipo)", "Clave de la pasarela integrada", "Tokens máximos por respuesta",
	"Arrancar la pasarela al abrir (s/n)", "URL de OmniRoute", "Clave de OmniRoute (Dashboard → Endpoints)", "URL de FreeLLMAPI", "Clave única de FreeLLMAPI (Keys)",
	"Instrucciones para el chat (prompt de sistema)",
}

func (m *model) openSettings() {
	c := m.cfg
	auto := "s"
	if !c.GatewayAutoRun {
		auto = "n"
	}
	vals := []string{itoa(c.GatewayPort), c.GatewayHost, c.GatewayKey, itoa(c.MaxTokensCap), auto,
		c.OmniRoute.URL, c.OmniRoute.APIKey, c.FreeLLMAPI.URL, c.FreeLLMAPI.APIKey, c.ChatSystem}
	secret := []bool{false, false, true, false, false, false, true, false, true, false}
	m.sform = newForm("sform", "Ajustes", settingsLabels, vals, secret, m.w)
}

func (m *model) keySettings(k tea.KeyMsg) tea.Cmd {
	if m.sform == nil {
		m.openSettings()
	}
	switch k.String() {
	case "esc":
		m.openSettings()
		m.setFlash("Cambios descartados")
		return nil
	case "up", "shift+tab":
		m.sform.move(-1)
		return nil
	case "down", "tab":
		m.sform.move(1)
		return nil
	case "ctrl+r":
		m.sform.toggleSecret()
		return nil
	case "enter", "ctrl+s":
		v := m.sform.values()
		c := m.cfg
		port, err := strconv.Atoi(v[0])
		if err != nil || port < 1 || port > 65535 {
			m.setFlash("Puerto no válido")
			return nil
		}
		maxTok, _ := strconv.Atoi(v[3])
		restart := port != c.GatewayPort || v[1] != c.GatewayHost
		c.mu.Lock()
		c.GatewayPort = port
		c.GatewayHost = v[1]
		c.GatewayKey = v[2]
		c.MaxTokensCap = maxTok
		c.GatewayAutoRun = !strings.HasPrefix(strings.ToLower(v[4]), "n")
		c.OmniRoute.URL, c.OmniRoute.Dashboard = v[5], strings.TrimSuffix(v[5], "/v1")
		c.OmniRoute.APIKey = v[6]
		c.FreeLLMAPI.URL, c.FreeLLMAPI.Dashboard = v[7], strings.TrimSuffix(v[7], "/v1")
		c.FreeLLMAPI.APIKey = v[8]
		c.ChatSystem = v[9]
		c.mu.Unlock()
		if err := c.Save(); err != nil {
			m.setFlash("No pude guardar: " + err.Error())
			return nil
		}
		if restart {
			if on, _ := m.gw.Running(); on {
				m.gw.Stop()
				_ = m.gw.Start()
			}
		}
		m.models = map[string][]string{}
		m.setFlash("Ajustes guardados")
		return m.checkBackends()
	}
	return m.sform.update(k)
}
