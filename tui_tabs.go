package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ======================= Inicio =======================

func (m *model) homeAction(what string) tea.Cmd {
	switch what {
	case "install":
		if !hasPack() {
			m.setFlash("Este binario es la versión ligera: descarga la versión completa del release para instalar sin internet")
			return nil
		}
		m.core.Install()
		m.setFlash("Instalando FreeLLMAPI…")
	case "start":
		if err := m.apps.Start("freellmapi"); err != nil {
			m.setFlash(err.Error())
			return nil
		}
		m.setFlash("FreeLLMAPI arrancando… tarda unos segundos")
	case "stop":
		m.apps.Stop("freellmapi")
		m.setFlash("FreeLLMAPI detenido")
	case "web":
		u := m.cfg.FreeLLMAPI.Dashboard
		if u == "" {
			u = m.cfg.FreeLLMAPI.URL
		}
		if err := openURL(u); err != nil {
			m.setFlash(err.Error())
		} else {
			m.setFlash("Abriendo " + u)
		}
	case "log":
		m.logOpen = true
		m.refreshLog()
		m.logVP.GotoBottom()
	case "files":
		return m.switchTab(tabFiles)
	case "chat":
		return m.switchTab(tabChat)
	}
	return m.refresh()
}

func (m *model) keyHome(s string) tea.Cmd {
	switch s {
	case "r":
		return m.refresh()
	case "i":
		return m.homeAction("install")
	case "l":
		return m.homeAction("log")
	}
	return nil
}

func progressBar(pct float64, w int) string {
	w = max(10, w)
	fill := int(pct * float64(w))
	return sOK.Render(strings.Repeat("█", fill)) + sDim.Render(strings.Repeat("░", w-fill))
}

func (m *model) viewHome() string {
	st := m.st
	var sb strings.Builder
	cw := min(m.w, 100)
	inner := cw - 4

	var state string
	switch {
	case st.Installing:
		state = sWarn.Render("● instalando…")
	case !st.Installed:
		state = sDim.Render("○ no instalado")
	case st.OK:
		state = sOK.Render(fmt.Sprintf("● en marcha · %d modelos", st.Models))
	case st.Running:
		state = sWarn.Render("● arrancando…")
	case st.Error != "":
		state = sBad.Render("● " + short(st.Error, 80))
	default:
		state = sDim.Render("○ parado")
	}
	content := sBold.Render("FreeLLMAPI") + "  " + state + "\n" +
		sDim.Render(truncate("Chat de IA gratis con muchos proveedores · panel web en "+st.Dashboard, inner*2)) + "\n"
	if st.Installing {
		content += progressBar(st.Progress, inner-6) + fmt.Sprintf(" %3.0f%%", st.Progress*100) + "\n"
	}
	if st.InstallErr != "" {
		content += sBad.Render(lipgloss.NewStyle().Width(inner).Render("La instalación falló: "+st.InstallErr)) + "\n"
	}
	if st.Outdated {
		content += sWarn.Render("Este binario trae una versión más nueva. Pulsa Actualizar (tus claves se conservan).") + "\n"
	}
	var btns []string
	switch {
	case st.Installing:
	case !st.Installed && st.HasPack:
		btns = append(btns, m.btn("home:install", "Instalar", sBtnPri))
	case !st.Installed:
		btns = append(btns, sDim.Render("No instalado: usa la versión completa del release"))
	case st.Running:
		btns = append(btns, m.btn("home:stop", "Parar", sBtnBad))
	case st.OK:
	default:
		btns = append(btns, m.btn("home:start", "Arrancar", sBtnPri))
	}
	if st.Outdated && !st.Installing {
		btns = append(btns, m.btn("home:install", "Actualizar", sBtnPri))
	}
	if st.Installed {
		btns = append(btns, m.btn("home:web", "Panel web", sBtn), m.btn("home:log", "Registro", sBtn))
	}
	btns = append(btns, m.btn("home:refresh", "Comprobar", sBtn))
	content += row(inner, btns...)
	cs := sCard
	if st.OK {
		cs = sCardOn
	}
	sb.WriteString(cs.Width(cw-2).Render(content) + "\n")

	chat := sBold.Render("Chat") + "  " + sDim.Render("modelo "+st.Model) + "\n" +
		sDim.Render(lipgloss.NewStyle().Width(inner).Render("Responde con FreeLLMAPI. Desde el chat puedes generar imágenes, vídeo y voz, y guardar respuestas, código o la conversación entera.")) + "\n" +
		row(inner, m.btn("home:chat", "Abrir el chat", sBtnPri), m.btn("tab:3", "Elegir modelo", sBtn))
	sb.WriteString(sCard.Width(cw-2).Render(chat) + "\n")

	files := sBold.Render("Archivos") + "\n" + sDim.Render(truncate(st.FilesDir, inner)) + "\n" +
		row(inner, m.btn("home:files", "Ver archivos", sBtn))
	sb.WriteString(sCard.Width(cw-2).Render(files) + "\n")
	if !st.OK && st.Installed && !st.Running && !st.Installing {
		sb.WriteString(sDim.Render(lipgloss.NewStyle().Width(cw).Render("FreeLLMAPI está parado: pulsa Arrancar. Se puede arrancar solo al abrir Infinito (Ajustes).")))
	}
	return sb.String()
}

// ======================= Chat =======================

var chatHelp = "Comandos: /imagen TEXTO · /video TEXTO · /voz TEXTO · /guardar · /codigo · /exportar · /modelo NOMBRE · /sistema TEXTO · /nuevo"

func (m *model) chatAction(what, arg string) tea.Cmd {
	switch what {
	case "send":
		return m.keyChat(tea.KeyMsg{Type: tea.KeyEnter})
	case "stop":
		m.core.Stop()
	case "new":
		return m.newChat()
	case "model":
		return m.switchTab(tabModels)
	case "focus":
		return m.ta.Focus()
	case "image", "video", "audio":
		text := strings.TrimSpace(m.ta.Value())
		if text == "" {
			what := map[string]string{"image": "la imagen", "video": "el vídeo", "audio": "lo que quieres oír"}[what]
			m.setFlash("Escribe primero en el cuadro " + what + " y pulsa este botón")
			return m.ta.Focus()
		}
		m.ta.Reset()
		return m.media(what, text)
	case "save":
		return m.saveAnswer(-1)
	case "saveidx":
		n, _ := strconv.Atoi(arg)
		return m.saveAnswer(n)
	case "code":
		names, err := m.core.SaveCode()
		if err != nil {
			m.setFlash(err.Error())
		} else {
			m.setFlash(fmt.Sprintf("Código guardado: %s (en %s)", strings.Join(names, ", "), filesDir()))
		}
	case "export":
		n, err := m.core.ExportChat()
		if err != nil {
			m.setFlash(err.Error())
		} else {
			m.setFlash("Conversación guardada: " + filepath.Join(filesDir(), n))
		}
	}
	return nil
}

func (m *model) saveAnswer(idx int) tea.Cmd {
	n, err := m.core.SaveAnswer(idx)
	if err != nil {
		m.setFlash(err.Error())
	} else {
		m.setFlash("Respuesta guardada: " + filepath.Join(filesDir(), n))
	}
	return nil
}

func (m *model) newChat() tea.Cmd {
	m.core.NewChat()
	m.partial, m.chatRoute = "", ""
	m.renderChat()
	return nil
}

func (m *model) media(kind, prompt string) tea.Cmd {
	if b := m.core.Busy(); b != "" {
		m.setFlash("Espera a que termine: " + b)
		return nil
	}
	c := m.core
	cmd := func() tea.Msg {
		_, err := c.Media(kind, prompt)
		return mediaDoneMsg{kind: kind, err: err}
	}
	label := map[string]string{"image": "la imagen", "video": "el vídeo (puede tardar minutos)", "audio": "la voz"}[kind]
	m.setFlash("Generando " + label + "…")
	return cmd
}

func (m *model) keyChat(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "enter":
		text := strings.TrimSpace(m.ta.Value())
		if text == "" {
			return nil
		}
		if strings.HasPrefix(text, "/") {
			m.ta.Reset()
			return m.chatCommand(text)
		}
		if b := m.core.Busy(); b != "" {
			m.setFlash("Espera a que termine: " + b + " (Esc para parar)")
			return nil
		}
		ch, err := m.core.Chat(text)
		if err != nil {
			m.setFlash(err.Error())
			return nil
		}
		m.ta.Reset()
		m.streaming = true
		m.partial, m.chatRoute = "", ""
		m.renderChat()
		return waitChat(ch)
	case "esc":
		m.core.Stop()
		return nil
	case "ctrl+n":
		return m.newChat()
	case "ctrl+o":
		return m.chatAction("model", "")
	case "ctrl+s":
		return m.saveAnswer(-1)
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
	rest := strings.TrimSpace(strings.TrimPrefix(cmd, f[0]))
	switch strings.ToLower(f[0]) {
	case "/imagen", "/image", "/img":
		return m.mediaCmd("image", rest)
	case "/video", "/vídeo":
		return m.mediaCmd("video", rest)
	case "/voz", "/audio", "/tts":
		return m.mediaCmd("audio", rest)
	case "/guardar", "/save":
		return m.saveAnswer(-1)
	case "/codigo", "/código", "/code":
		return m.chatAction("code", "")
	case "/exportar", "/export":
		return m.chatAction("export", "")
	case "/modelo", "/model":
		if rest == "" {
			return m.switchTab(tabModels)
		}
		m.core.SetModel(rest)
		m.setFlash("Modelo: " + rest)
		m.mlist = nil
	case "/sistema", "/system":
		m.cfg.mu.Lock()
		m.cfg.ChatSystem = rest
		m.cfg.mu.Unlock()
		_ = m.cfg.Save()
		m.setFlash("Instrucciones del chat actualizadas")
	case "/nuevo", "/new", "/clear":
		return m.newChat()
	case "/archivos", "/files":
		return m.switchTab(tabFiles)
	default:
		m.setFlash(chatHelp)
	}
	return nil
}

func (m *model) mediaCmd(kind, prompt string) tea.Cmd {
	if prompt == "" {
		m.setFlash("Escribe qué quieres después del comando, por ejemplo: /imagen un gato astronauta")
		return nil
	}
	return m.media(kind, prompt)
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
	if ev.Route != "" {
		m.chatRoute = ev.Route
	}
	m.partial += ev.Delta
	if msg.ch == nil {
		// El canal se cerró: Core ya guardó la respuesta.
		m.streaming = false
		m.partial = ""
		m.renderChat()
		return nil
	}
	m.renderChat()
	return waitChat(msg.ch)
}

func (m *model) renderChat() {
	w := max(20, m.vp.Width-1)
	wrap := lipgloss.NewStyle().Width(w)
	hist := m.core.History()
	m.histLen = len(hist)
	var sb strings.Builder
	if len(hist) == 0 && !m.streaming {
		sb.WriteString(sDim.Render(wrap.Render("Escribe abajo y pulsa Enter (o Enviar). Responde FreeLLMAPI.\n\n" +
			"Para imágenes, vídeo o voz escribe la descripción y pulsa 🖼 Imagen, 🎬 Vídeo o 🔊 Voz. " +
			"Con 💾 guardas la última respuesta, con </> su código en archivos y con 📄 toda la conversación.\n\n" + chatHelp)))
	}
	for i, h := range hist {
		switch {
		case h.Role == "user":
			sb.WriteString(sUser.Render("Tú") + "\n" + wrap.Render(h.Content) + "\n\n")
		case h.Kind == "error":
			sb.WriteString(sBad.Render("Error") + "\n" + sBad.Render(wrap.Render(h.Content)) + "\n" +
				sDim.Render(wrap.Render(errHint(h.Content))) + "\n\n")
		case len(h.Files) > 0:
			sb.WriteString(sBot.Render("IA") + sDim.Render(" · "+h.Via) + "\n")
			for _, f := range h.Files {
				icon := map[string]string{"image": "🖼", "video": "🎬", "audio": "🔊"}[fileKind(f)]
				if icon == "" {
					icon = "📎"
				}
				sb.WriteString(icon + " " + f + "  " + m.btn("file:open:"+f, "Abrir", sBtnPri) + "\n")
			}
			sb.WriteString("\n")
		default:
			save := m.btn(fmt.Sprintf("chat:saveidx:%d", i), "💾", sBtn)
			sb.WriteString(sBot.Render("IA") + sDim.Render(" · "+h.Via) + " " + save + "\n" + wrap.Render(h.Content) + "\n\n")
		}
	}
	if m.streaming {
		via := "FreeLLMAPI"
		if m.chatRoute != "" {
			via += " · " + m.chatRoute
		}
		sb.WriteString(sBot.Render("IA") + sDim.Render(" · "+via+" · escribiendo…") + "\n" + wrap.Render(m.partial+"▌") + "\n")
	} else if b := m.core.Busy(); b != "" {
		sb.WriteString(sWarn.Render("⏳ Generando "+b+"…") + "\n")
	}
	m.vp.SetContent(sb.String())
	m.vp.GotoBottom()
}

func (m *model) viewChat(height int) string {
	model := m.core.model()
	state := sOK.Render("●")
	if !m.st.OK {
		state = sBad.Render("●")
	}
	top := row(m.w, state+" "+sDim.Render("FreeLLMAPI"), m.btn("chat:model:", "Modelo: "+truncate(model, 24)+" ▾", sBtn))
	var actions []string
	if m.streaming || m.core.Busy() != "" {
		actions = append(actions, m.btn("chat:stop:", "■ Parar", sBtnBad))
	} else {
		actions = append(actions, m.btn("chat:send:", "Enviar", sBtnPri))
	}
	actions = append(actions,
		m.btn("chat:image:", "🖼 Imagen", sBtn),
		m.btn("chat:video:", "🎬 Vídeo", sBtn),
		m.btn("chat:audio:", "🔊 Voz", sBtn),
		m.btn("chat:save:", "💾 Guardar", sBtn),
		m.btn("chat:code:", "</> Código", sBtn),
		m.btn("chat:export:", "📄 Exportar", sBtn),
		m.btn("chat:new:", "Nuevo", sBtn),
	)
	bottom := row(m.w, actions...)
	// Los botones del historial (Abrir, 💾) se registran al dibujar el visor.
	m.renderChatKeepScroll()
	m.vp.Height = max(3, height-lipgloss.Height(top)-lipgloss.Height(bottom)-m.ta.Height()-1)
	return top + "\n" + m.vp.View() + "\n" + m.mark("chat:focus:", m.ta.View()) + "\n" + bottom
}

// renderChatKeepScroll vuelve a dibujar el historial sin mover el visor, para
// que los botones del historial queden registrados en esta vista.
func (m *model) renderChatKeepScroll() {
	off := m.vp.YOffset
	bottom := m.vp.AtBottom()
	m.renderChat()
	if !bottom {
		m.vp.SetYOffset(off)
	}
}

// ======================= Archivos =======================

func (m *model) fileAction(what, name string) tea.Cmd {
	if name == "" && m.fcursor < len(m.files) {
		name = m.files[m.fcursor].Name
	}
	if what == "reload" {
		m.files = ListFiles()
		return nil
	}
	if what == "folder" {
		if err := openURL(filesDir()); err != nil {
			m.setFlash(err.Error())
		}
		return nil
	}
	if name == "" {
		return nil
	}
	p, err := FilePath(name)
	if err != nil {
		m.setFlash(err.Error())
		return nil
	}
	switch what {
	case "open":
		if err := openURL(p); err != nil {
			m.setFlash(err.Error())
		} else {
			m.setFlash("Abriendo " + name)
		}
	case "copy":
		copyClipboard(p)
		m.setFlash("Ruta copiada: " + p)
	case "del":
		if m.confirmDel != name {
			m.confirmDel = name
			m.setFlash("Pulsa Borrar otra vez para borrar " + name)
			return nil
		}
		m.confirmDel = ""
		if err := DeleteFile(name); err != nil {
			m.setFlash(err.Error())
		} else {
			m.setFlash("Borrado " + name)
		}
		m.files = ListFiles()
		m.fcursor = min(m.fcursor, max(0, len(m.files)-1))
	}
	return nil
}

func (m *model) keyFiles(s string) tea.Cmd {
	switch s {
	case "up", "k":
		m.fcursor = max(0, m.fcursor-1)
		m.confirmDel = ""
	case "down", "j":
		m.fcursor = min(max(0, len(m.files)-1), m.fcursor+1)
		m.confirmDel = ""
	case "enter", "o":
		return m.fileAction("open", "")
	case "d", "delete":
		return m.fileAction("del", "")
	case "c":
		return m.fileAction("copy", "")
	case "r":
		return m.fileAction("reload", "")
	}
	return nil
}

func (m *model) viewFiles(height int) string {
	head := sDim.Render(truncate("Carpeta: "+filesDir(), m.w)) + "\n" +
		row(m.w, m.btn("file:open:", "Abrir", sBtnPri), m.btn("file:copy:", "Copiar ruta", sBtn),
			m.btn("file:del:", "Borrar", sBtnBad), m.btn("file:reload:", "Recargar", sBtn), m.btn("file:folder:", "Abrir carpeta", sBtn))
	if len(m.files) == 0 {
		return head + "\n\n" + sDim.Render(lipgloss.NewStyle().Width(m.w).Render("Todavía no hay archivos. En el chat usa 🖼 Imagen, 🎬 Vídeo, 🔊 Voz, 💾 Guardar, </> Código o 📄 Exportar."))
	}
	m.fcursor = min(m.fcursor, len(m.files)-1)
	rows := max(3, height-lipgloss.Height(head)-1)
	start := 0
	if m.fcursor >= rows {
		start = m.fcursor - rows + 1
	}
	var sb strings.Builder
	for i := start; i < len(m.files) && i < start+rows; i++ {
		f := m.files[i]
		icon := map[string]string{"image": "🖼", "video": "🎬", "audio": "🔊"}[f.Kind]
		if icon == "" {
			icon = "📄"
		}
		meta := f.Time.Format("02/01 15:04") + " · " + humanSize(f.Size)
		nameW := max(10, m.w-lipgloss.Width(meta)-8)
		name := fmt.Sprintf("%-*s", nameW, truncate(f.Name, nameW))
		mark := "  "
		if i == m.fcursor {
			mark = sSel.Render("▸ ")
			name = sSel.Render(name)
		}
		sb.WriteString(m.mark(fmt.Sprintf("frow:%d", i), mark+icon+" "+name+" "+sDim.Render(meta)) + "\n")
	}
	return head + "\n" + sb.String()
}

// ======================= Modelos =======================

func (m *model) keyModels(k tea.KeyMsg) tea.Cmd {
	if m.mlist != nil {
		if ok, c := m.mlist.handleKey(k, m.h-10); ok {
			return c
		}
	}
	switch k.String() {
	case "r":
		m.mlist = nil
		return m.loadModels()
	case "enter":
		if m.mlist == nil || m.mlist.current() == "" {
			return nil
		}
		m.core.SetModel(m.mlist.current())
		m.setFlash("El chat usará " + m.mlist.current())
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
	acts := row(m.w, m.btn("mact:filter", "Buscar", sBtn), m.btn("mact:reload", "Recargar", sBtn), m.btn("mact:copy", "Copiar nombre", sBtn))
	intro := sDim.Render(lipgloss.NewStyle().Width(m.w).Render("En uso: " + m.core.model() + ". «auto» deja que FreeLLMAPI elija el mejor proveedor libre."))
	out := intro + "\n" + acts + "\n"
	listH := height - lipgloss.Height(intro) - lipgloss.Height(acts) - 2
	switch {
	case m.modelsLoading && m.mlist == nil:
		out += sDim.Render("Cargando modelos de FreeLLMAPI…")
	case m.modelsErr != "":
		out += sBad.Render(lipgloss.NewStyle().Width(m.w).Render("No pude leer los modelos: "+m.modelsErr)) + "\n\n" +
			sDim.Render(lipgloss.NewStyle().Width(m.w).Render(errHint(m.modelsErr)))
	case m.mlist != nil:
		out += m.pickView(m.mlist, listH)
	}
	return out
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

func (m *model) viewGuides(height int) string {
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
		m.gvp.Height = max(3, height-lipgloss.Height(top)-1)
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
	"Al abrir Infinito (preguntar / gui / tui)",
	"Arrancar FreeLLMAPI al abrir (s/n)",
	"URL de FreeLLMAPI",
	"Clave única de FreeLLMAPI (se rellena sola)",
	"Modelo del chat (auto = el mejor libre)",
	"Instrucciones para el chat (prompt de sistema)",
	"Puerto de la interfaz gráfica",
}

func (m *model) openSettings() {
	c := m.cfg
	c.mu.RLock()
	defer c.mu.RUnlock()
	mode := c.StartMode
	if mode == "" {
		mode = "preguntar"
	}
	fauto := "s"
	if c.FreeLLMAPIManual {
		fauto = "n"
	}
	vals := []string{mode, fauto, c.FreeLLMAPI.URL, c.FreeLLMAPI.APIKey, c.ChatModel, c.ChatSystem, itoa(c.GUIPort)}
	secret := []bool{false, false, false, true, false, false, false}
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
		mode := strings.ToLower(v[0])
		switch {
		case strings.HasPrefix(mode, "g"):
			mode = "gui"
		case strings.HasPrefix(mode, "t"):
			mode = "tui"
		default:
			mode = ""
		}
		port, err := strconv.Atoi(v[6])
		if err != nil || port < 1 || port > 65535 {
			m.setFlash("Puerto no válido")
			return nil
		}
		c := m.cfg
		c.mu.Lock()
		c.StartMode = mode
		c.FreeLLMAPIManual = strings.HasPrefix(strings.ToLower(v[1]), "n")
		u := strings.TrimSuffix(strings.TrimRight(v[2], "/"), "/v1")
		c.FreeLLMAPI.URL, c.FreeLLMAPI.Dashboard = u, u
		c.FreeLLMAPI.APIKey = v[3]
		c.ChatModel = v[4]
		if c.ChatModel == "" {
			c.ChatModel = "auto"
		}
		c.ChatSystem = v[5]
		c.GUIPort = port
		c.mu.Unlock()
		if err := c.Save(); err != nil {
			m.setFlash("No pude guardar: " + err.Error())
			return nil
		}
		m.mlist = nil
		m.setFlash("Ajustes guardados")
		return m.refresh()
	}
	return m.sform.update(k)
}
