package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Core es la parte común de la GUI y la TUI: instala y arranca FreeLLMAPI,
// guarda la conversación y habla con su API (texto, imágenes, vídeo y voz).
type Core struct {
	cfg  *Config
	apps *AppManager

	mu         sync.Mutex
	history    []storedMsg
	installing bool
	installPr  *progress
	installErr string
	ok         bool // FreeLLMAPI responde
	nModels    int
	lastErr    string
	checked    time.Time
	busy       string // "chat", "imagen"… mientras hay una petición en curso
	cancel     context.CancelFunc
}

// storedMsg es un mensaje de la conversación tal como se guarda en disco.
type storedMsg struct {
	chatMsg
	Via   string   `json:"via,omitempty"`
	Files []string `json:"files,omitempty"` // nombres dentro de filesDir()
	Kind  string   `json:"kind,omitempty"`  // "", "image", "video", "audio", "error"
}

func NewCore(cfg *Config, apps *AppManager) *Core {
	c := &Core{cfg: cfg, apps: apps}
	c.loadHistory()
	return c
}

// ---------- Conversación ----------

func historyPath() string { return filepath.Join(configDir(), "chat.json") }

func (c *Core) loadHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = nil
	if b, err := os.ReadFile(historyPath()); err == nil {
		_ = json.Unmarshal(b, &c.history)
	}
}

func (c *Core) saveLocked() {
	b, _ := json.MarshalIndent(c.history, "", " ")
	_ = os.MkdirAll(configDir(), 0o700)
	_ = os.WriteFile(historyPath(), b, 0o600)
}

func (c *Core) History() []storedMsg {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]storedMsg(nil), c.history...)
}

func (c *Core) append(m storedMsg) {
	c.mu.Lock()
	c.history = append(c.history, m)
	c.saveLocked()
	c.mu.Unlock()
}

func (c *Core) NewChat() {
	c.Stop()
	c.mu.Lock()
	c.history = nil
	c.saveLocked()
	c.mu.Unlock()
}

// Busy dice qué se está generando ("" si nada).
func (c *Core) Busy() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy
}

func (c *Core) begin(what string) (context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy != "" {
		return nil, fmt.Errorf("espera a que termine: %s", c.busy)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.busy, c.cancel = what, cancel
	return ctx, nil
}

func (c *Core) end() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.busy, c.cancel = "", nil
	c.mu.Unlock()
}

// Stop cancela lo que se esté generando.
func (c *Core) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
}

func (c *Core) endpoint() (base, key string) {
	c.cfg.mu.RLock()
	defer c.cfg.mu.RUnlock()
	return v1(c.cfg.FreeLLMAPI.URL), c.cfg.FreeLLMAPI.APIKey
}

func (c *Core) model() string {
	c.cfg.mu.RLock()
	defer c.cfg.mu.RUnlock()
	if c.cfg.ChatModel == "" {
		return "auto"
	}
	return c.cfg.ChatModel
}

// Chat envía el texto y emite la respuesta a trozos. Al terminar la guarda.
func (c *Core) Chat(text string) (<-chan chatEvent, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("escribe algo")
	}
	ctx, err := c.begin("respuesta")
	if err != nil {
		return nil, err
	}
	c.append(storedMsg{chatMsg: chatMsg{Role: "user", Content: text}})
	var msgs []chatMsg
	c.cfg.mu.RLock()
	sys := c.cfg.ChatSystem
	c.cfg.mu.RUnlock()
	if sys != "" {
		msgs = append(msgs, chatMsg{Role: "system", Content: sys})
	}
	for _, h := range c.History() {
		if h.Kind == "error" || h.Content == "" {
			continue
		}
		msgs = append(msgs, h.chatMsg)
	}
	base, key := c.endpoint()
	in := chatStream(ctx, base, key, c.model(), msgs)
	out := make(chan chatEvent, 64)
	go func() {
		defer close(out)
		defer c.end()
		var sb strings.Builder
		route := ""
		for ev := range in {
			if ev.Route != "" {
				route = ev.Route
			}
			sb.WriteString(ev.Delta)
			if ev.Err != nil {
				if ctx.Err() == nil {
					c.append(storedMsg{chatMsg: chatMsg{Role: "assistant", Content: ev.Err.Error()}, Kind: "error"})
				}
			}
			out <- ev
		}
		if sb.Len() > 0 {
			via := "FreeLLMAPI"
			if route != "" {
				via += " · " + route
			}
			c.append(storedMsg{chatMsg: chatMsg{Role: "assistant", Content: strings.TrimSpace(sb.String())}, Via: via})
		}
	}()
	return out, nil
}

// ---------- Multimedia ----------

// Media genera una imagen ("image"), un vídeo ("video") o voz ("audio") con
// FreeLLMAPI y guarda el resultado en la carpeta de archivos.
func (c *Core) Media(kind, prompt string) (storedMsg, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return storedMsg{}, fmt.Errorf("escribe qué quieres generar")
	}
	label := map[string]string{"image": "imagen", "video": "vídeo", "audio": "voz"}[kind]
	if label == "" {
		return storedMsg{}, fmt.Errorf("tipo desconocido: %s", kind)
	}
	ctx, err := c.begin(label)
	if err != nil {
		return storedMsg{}, err
	}
	defer c.end()
	prefix := map[string]string{"image": "🖼 Imagen: ", "video": "🎬 Vídeo: ", "audio": "🔊 Voz: "}[kind]
	c.append(storedMsg{chatMsg: chatMsg{Role: "user", Content: prefix + prompt}, Kind: kind})
	files, via, err := c.generate(ctx, kind, prompt)
	if err != nil {
		if ctx.Err() == nil {
			c.append(storedMsg{chatMsg: chatMsg{Role: "assistant", Content: err.Error()}, Kind: "error"})
		}
		return storedMsg{}, err
	}
	m := storedMsg{chatMsg: chatMsg{Role: "assistant", Content: ""}, Files: files, Kind: kind, Via: via}
	// El modelo no ve los archivos: le dejamos una nota para que la charla siga con sentido.
	m.Content = fmt.Sprintf("[%s: archivo generado %s]", label, strings.Join(files, ", "))
	c.append(m)
	return m, nil
}

func (c *Core) generate(ctx context.Context, kind, prompt string) ([]string, string, error) {
	base, key := c.endpoint()
	var path string
	var body map[string]any
	switch kind {
	case "image":
		path, body = "/images/generations", map[string]any{"prompt": prompt, "n": 1}
	case "video":
		path, body = "/videos/generations", map[string]any{"prompt": prompt}
	case "audio":
		path, body = "/audio/speech", map[string]any{"input": prompt, "response_format": "mp3"}
	}
	b, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", apiError(resp.StatusCode, data)
	}
	ct := resp.Header.Get("Content-Type")
	via := "FreeLLMAPI"
	if p := resp.Header.Get("X-Provider"); p != "" {
		via += " · " + p
	}
	stem := map[string]string{"image": "imagen", "video": "video", "audio": "voz"}[kind]
	if kind != "image" || !strings.Contains(ct, "json") {
		name, err := saveFile(stem, extFor(ct, kind), data)
		if err != nil {
			return nil, "", err
		}
		return []string{name}, via, nil
	}
	var r struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal(data, &r); err != nil || len(r.Data) == 0 {
		return nil, "", fmt.Errorf("respuesta no válida: %s", short(string(data), 200))
	}
	if r.Provider != "" {
		via = "FreeLLMAPI · " + r.Provider
	}
	var names []string
	for _, d := range r.Data {
		img, ext, err := fetchImage(ctx, d.URL, d.B64JSON)
		if err != nil {
			return nil, "", err
		}
		name, err := saveFile(stem, ext, img)
		if err != nil {
			return nil, "", err
		}
		names = append(names, name)
	}
	return names, via, nil
}

func apiError(status int, b []byte) error {
	msg := string(b)
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error.Message != "" {
		msg = e.Error.Message
	}
	return fmt.Errorf("HTTP %d: %s", status, short(msg, 400))
}

func fetchImage(ctx context.Context, url, b64 string) ([]byte, string, error) {
	switch {
	case b64 != "":
		d, err := base64.StdEncoding.DecodeString(b64)
		return d, sniffExt(d, ".png"), err
	case strings.HasPrefix(url, "data:"):
		meta, payload, _ := strings.Cut(strings.TrimPrefix(url, "data:"), ",")
		var d []byte
		var err error
		if strings.HasSuffix(meta, ";base64") {
			d, err = base64.StdEncoding.DecodeString(payload)
		} else {
			d = []byte(payload)
		}
		return d, extFor(strings.TrimSuffix(meta, ";base64"), "image"), err
	case url != "":
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := httpc.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		d, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err == nil && resp.StatusCode/100 != 2 {
			err = fmt.Errorf("no pude descargar la imagen: HTTP %d", resp.StatusCode)
		}
		return d, extFor(resp.Header.Get("Content-Type"), "image"), err
	}
	return nil, "", fmt.Errorf("la respuesta no trae imagen")
}

func sniffExt(d []byte, def string) string {
	if e := extFor(http.DetectContentType(d), ""); e != "" {
		return e
	}
	return def
}

func extFor(ct, kind string) string {
	ct, _, _ = mime.ParseMediaType(ct)
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/aac":
		return ".aac"
	case "text/markdown":
		return ".md"
	}
	switch kind {
	case "image":
		return ".png"
	case "video":
		return ".mp4"
	case "audio":
		return ".mp3"
	}
	return ""
}

// ---------- Archivos ----------

// filesDir es donde se guardan las respuestas, el código y lo multimedia.
func filesDir() string {
	if d := os.Getenv("INFINITO_FILES"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	if isTermux() {
		if st, err := os.Stat(filepath.Join(h, "storage", "downloads")); err == nil && st.IsDir() {
			return filepath.Join(h, "storage", "downloads", "Infinito")
		}
	}
	if runtime.GOOS == "windows" {
		if st, err := os.Stat(filepath.Join(h, "Documents")); err == nil && st.IsDir() {
			return filepath.Join(h, "Documents", "Infinito")
		}
	}
	return filepath.Join(h, "Infinito")
}

func saveFile(stem, ext string, data []byte) (string, error) {
	dir := filesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := stem + "-" + time.Now().Format("20060102-150405")
	name := base + ext
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	return name, os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

// FilePath da la ruta completa de un archivo guardado (solo nombres simples).
func FilePath(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("nombre no válido")
	}
	return filepath.Join(filesDir(), name), nil
}

type FileInfo struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	Time time.Time `json:"time"`
	Kind string    `json:"kind"`
}

func fileKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return "image"
	case ".mp4", ".webm":
		return "video"
	case ".mp3", ".wav", ".ogg", ".flac", ".aac":
		return "audio"
	}
	return "text"
}

func ListFiles() []FileInfo {
	ents, _ := os.ReadDir(filesDir())
	var out []FileInfo
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, FileInfo{Name: e.Name(), Size: info.Size(), Time: info.ModTime(), Kind: fileKind(e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

func DeleteFile(name string) error {
	p, err := FilePath(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// lastAnswer devuelve la última respuesta de texto de la IA.
func (c *Core) lastAnswer() (string, error) {
	h := c.History()
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].Role == "assistant" && h[i].Kind == "" && h[i].Content != "" {
			return h[i].Content, nil
		}
	}
	return "", fmt.Errorf("todavía no hay ninguna respuesta que guardar")
}

// SaveAnswer guarda una respuesta (la última si idx < 0) como Markdown.
func (c *Core) SaveAnswer(idx int) (string, error) {
	var text string
	if idx < 0 {
		t, err := c.lastAnswer()
		if err != nil {
			return "", err
		}
		text = t
	} else {
		h := c.History()
		if idx >= len(h) || h[idx].Content == "" {
			return "", fmt.Errorf("mensaje no válido")
		}
		text = h[idx].Content
	}
	return saveFile("respuesta", ".md", []byte(text+"\n"))
}

var fence = regexp.MustCompile("(?s)```([A-Za-z0-9_+#.-]*)[^\\n]*\\n(.*?)```")

var langExt = map[string]string{
	"go": ".go", "python": ".py", "py": ".py", "js": ".js", "javascript": ".js", "ts": ".ts", "typescript": ".ts",
	"html": ".html", "css": ".css", "json": ".json", "bash": ".sh", "sh": ".sh", "shell": ".sh", "zsh": ".sh",
	"powershell": ".ps1", "ps1": ".ps1", "bat": ".bat", "java": ".java", "kotlin": ".kt", "kt": ".kt", "c": ".c",
	"cpp": ".cpp", "c++": ".cpp", "csharp": ".cs", "cs": ".cs", "rust": ".rs", "rs": ".rs", "php": ".php",
	"ruby": ".rb", "rb": ".rb", "sql": ".sql", "yaml": ".yaml", "yml": ".yaml", "toml": ".toml", "xml": ".xml",
	"markdown": ".md", "md": ".md", "csv": ".csv", "svg": ".svg", "swift": ".swift", "lua": ".lua", "dart": ".dart",
	"txt": ".txt", "text": ".txt", "ini": ".ini", "dockerfile": ".dockerfile", "makefile": ".mk",
}

// SaveCode guarda cada bloque de código de la última respuesta en su archivo.
func (c *Core) SaveCode() ([]string, error) {
	t, err := c.lastAnswer()
	if err != nil {
		return nil, err
	}
	ms := fence.FindAllStringSubmatch(t, -1)
	if len(ms) == 0 {
		return nil, fmt.Errorf("la última respuesta no tiene bloques de código")
	}
	var names []string
	for _, m := range ms {
		ext := langExt[strings.ToLower(m[1])]
		if ext == "" {
			ext = ".txt"
		}
		n, err := saveFile("codigo", ext, []byte(m[2]))
		if err != nil {
			return names, err
		}
		names = append(names, n)
	}
	return names, nil
}

// ExportChat guarda toda la conversación en Markdown.
func (c *Core) ExportChat() (string, error) {
	h := c.History()
	if len(h) == 0 {
		return "", fmt.Errorf("la conversación está vacía")
	}
	var sb strings.Builder
	sb.WriteString("# Conversación con Infinito\n\n_" + time.Now().Format("02/01/2006 15:04") + "_\n\n")
	for _, m := range h {
		who := "**Tú**"
		if m.Role == "assistant" {
			who = "**IA**"
			if m.Via != "" {
				who += " _(" + m.Via + ")_"
			}
		}
		sb.WriteString(who + "\n\n")
		if len(m.Files) > 0 {
			for _, f := range m.Files {
				if fileKind(f) == "image" {
					sb.WriteString("![" + f + "](" + f + ")\n\n")
				} else {
					sb.WriteString("[" + f + "](" + f + ")\n\n")
				}
			}
		} else {
			sb.WriteString(m.Content + "\n\n")
		}
	}
	return saveFile("conversacion", ".md", []byte(sb.String()))
}

// ---------- FreeLLMAPI ----------

// Boot instala el paquete la primera vez y arranca FreeLLMAPI.
func (c *Core) Boot() {
	if !c.apps.Installed("freellmapi") {
		if hasPack() {
			c.Install()
		}
		return
	}
	if !c.cfg.FreeLLMAPIManual {
		_ = c.apps.Start("freellmapi")
	}
}

// Install descomprime el paquete en segundo plano y luego arranca FreeLLMAPI.
func (c *Core) Install() {
	c.mu.Lock()
	if c.installing {
		c.mu.Unlock()
		return
	}
	c.installing, c.installErr, c.installPr = true, "", &progress{}
	pr := c.installPr
	c.mu.Unlock()
	c.apps.StopAll()
	go func() {
		err := installPack(pr, nil)
		c.mu.Lock()
		c.installing = false
		if err != nil {
			c.installErr = err.Error()
		}
		c.mu.Unlock()
		if err == nil && !c.cfg.FreeLLMAPIManual {
			_ = c.apps.Start("freellmapi")
		}
	}()
}

// Status es el estado que muestran la GUI y la TUI.
type Status struct {
	Installed  bool    `json:"installed"`
	Running    bool    `json:"running"`
	OK         bool    `json:"ok"`
	Models     int     `json:"models"`
	Error      string  `json:"error,omitempty"`
	Installing bool    `json:"installing"`
	Progress   float64 `json:"progress"`
	InstallErr string  `json:"install_error,omitempty"`
	HasPack    bool    `json:"has_pack"`
	Outdated   bool    `json:"outdated"`
	Busy       string  `json:"busy,omitempty"`
	Dashboard  string  `json:"dashboard"`
	Model      string  `json:"model"`
	FilesDir   string  `json:"files_dir"`
	Version    string  `json:"version"`
}

// Refresh comprueba si FreeLLMAPI responde (como mucho cada 2 s).
func (c *Core) Refresh() {
	c.mu.Lock()
	if time.Since(c.checked) < 2*time.Second {
		c.mu.Unlock()
		return
	}
	c.checked = time.Now()
	c.mu.Unlock()
	base, key := c.endpoint()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	l, err := listModels(ctx, base, key)
	c.mu.Lock()
	c.ok, c.nModels, c.lastErr = err == nil, len(l), ""
	if err != nil {
		c.lastErr = err.Error()
	}
	c.mu.Unlock()
}

func (c *Core) Status() Status {
	installed := c.apps.Installed("freellmapi")
	running := c.apps.Running("freellmapi")
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Status{
		Installed: installed, Running: running, OK: c.ok, Models: c.nModels,
		Installing: c.installing, InstallErr: c.installErr, HasPack: hasPack(),
		Busy: c.busy, Dashboard: c.cfg.FreeLLMAPI.Dashboard, Model: c.cfg.ChatModel,
		FilesDir: filesDir(), Version: version,
	}
	if s.Model == "" {
		s.Model = "auto"
	}
	if !c.ok && c.lastErr != "" && running {
		s.Error = c.lastErr
	}
	if !running && !c.ok {
		if err := c.apps.LastError("freellmapi"); err != nil {
			s.Error = err.Error()
		}
	}
	if c.installing && c.installPr != nil {
		if t := c.installPr.total.Load(); t > 0 {
			s.Progress = float64(c.installPr.done.Load()) / float64(t)
		}
	}
	if installed && s.HasPack && !c.installing {
		s.Outdated = packOutdated()
	}
	return s
}

// Models lista los modelos de chat de FreeLLMAPI.
func (c *Core) Models() ([]string, error) {
	base, key := c.endpoint()
	return listModels(context.Background(), base, key)
}

func (c *Core) SetModel(m string) {
	c.cfg.mu.Lock()
	c.cfg.ChatModel = m
	c.cfg.mu.Unlock()
	_ = c.cfg.Save()
}

// errHint explica qué hacer cuando FreeLLMAPI no consigue responder.
func errHint(err string) string {
	e := strings.ToLower(err)
	switch {
	case strings.Contains(e, "connection refused") || strings.Contains(e, "connectex") || strings.Contains(e, "no such host"):
		return "FreeLLMAPI no está en marcha. Ve a Inicio y pulsa Arrancar."
	case strings.Contains(e, "401") || strings.Contains(e, "invalid api key"):
		return "Falta la clave de FreeLLMAPI. Para y vuelve a arrancar FreeLLMAPI en Inicio: Infinito la copia sola."
	case strings.Contains(e, "provider") || strings.Contains(e, "exhausted") || strings.Contains(e, "api key") || strings.Contains(e, "no_") || strings.Contains(e, "503"):
		return "Ningún proveedor gratuito de FreeLLMAPI ha respondido. Abre el panel de FreeLLMAPI (Inicio → Panel web) y añade una clave gratuita, por ejemplo de Groq, Gemini u OpenRouter."
	}
	return "Revisa Inicio: FreeLLMAPI tiene que estar en marcha."
}
