package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Interfaz gráfica: una app web local (estilo app Android moderna) que sirve
// el propio binario. En el PC se abre en el navegador; en el APK, a pantalla
// completa dentro de la app. Comparte la conversación con la TUI.

//go:embed gui/index.html
var guiFS embed.FS

type GUIServer struct {
	cfg   *Config
	core  *Core
	token string
	srv   *http.Server
}

func NewGUIServer(cfg *Config, core *Core) *GUIServer {
	tok := os.Getenv("INFINITO_GUI_TOKEN")
	if tok == "" {
		tok = randHex(16)
	}
	return &GUIServer{cfg: cfg, core: core, token: tok}
}

// Start escucha en 127.0.0.1 y devuelve la URL que hay que abrir.
func (g *GUIServer) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+itoa(g.cfg.GUIPort))
	if err != nil {
		return "", err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/app/", http.StatusFound)
	})
	mux.HandleFunc("/app/", g.index)
	mux.HandleFunc("/app/files/", g.auth(g.serveFile))
	mux.HandleFunc("/app/api/", g.auth(g.api))
	g.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = g.srv.Serve(ln) }()
	return g.cfg.guiURL() + "/app/#t=" + g.token, nil
}

func (g *GUIServer) Stop() {
	if g.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = g.srv.Shutdown(ctx)
	}
}

func localHost(h string) bool {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return h == "127.0.0.1" || h == "localhost" || h == "::1" || h == "[::1]"
}

func (g *GUIServer) index(w http.ResponseWriter, r *http.Request) {
	if !localHost(r.Host) {
		http.Error(w, "solo local", http.StatusForbidden)
		return
	}
	b, _ := guiFS.ReadFile("gui/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
	_, _ = w.Write(b)
}

func (g *GUIServer) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := r.Header.Get("X-Token")
		if t == "" {
			t = r.URL.Query().Get("t")
		}
		if !localHost(r.Host) || t != g.token {
			http.Error(w, "no autorizado", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (g *GUIServer) serveFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/app/files/")
	p, err := FilePath(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("dl") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "hint": errHint(err.Error())})
}

type settingsJSON struct {
	StartMode string `json:"start_mode"`
	Autostart bool   `json:"autostart"`
	URL       string `json:"url"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	System    string `json:"system"`
	GUIPort   int    `json:"gui_port"`
}

func (g *GUIServer) api(w http.ResponseWriter, r *http.Request) {
	what := strings.TrimPrefix(r.URL.Path, "/app/api/")
	var in struct {
		Text   string        `json:"text"`
		Kind   string        `json:"kind"`
		Prompt string        `json:"prompt"`
		Idx    *int          `json:"idx"`
		Name   string        `json:"name"`
		Model  string        `json:"model"`
		What   string        `json:"what"`
		Set    *settingsJSON `json:"settings"`
	}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&in); err != nil && !errors.Is(err, context.Canceled) && err.Error() != "EOF" {
			writeErr(w, err)
			return
		}
	} else if what != "state" && what != "history" && what != "files" && what != "models" && what != "settings" && what != "log" {
		http.Error(w, "usa POST", http.StatusMethodNotAllowed)
		return
	}
	c := g.core
	switch what {
	case "state":
		go c.Refresh()
		writeJSON(w, c.Status())
	case "history":
		h := c.History()
		if h == nil {
			h = []storedMsg{}
		}
		writeJSON(w, h)
	case "chat":
		g.chat(w, r, in.Text)
	case "stop":
		c.Stop()
		writeJSON(w, map[string]bool{"ok": true})
	case "new":
		c.NewChat()
		writeJSON(w, map[string]bool{"ok": true})
	case "media":
		m, err := c.Media(in.Kind, in.Prompt)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, m)
	case "save":
		idx := -1
		if in.Idx != nil {
			idx = *in.Idx
		}
		n, err := c.SaveAnswer(idx)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"names": []string{n}, "dir": filesDir()})
	case "code":
		ns, err := c.SaveCode()
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"names": ns, "dir": filesDir()})
	case "export":
		n, err := c.ExportChat()
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"names": []string{n}, "dir": filesDir()})
	case "files":
		l := ListFiles()
		if l == nil {
			l = []FileInfo{}
		}
		writeJSON(w, map[string]any{"files": l, "dir": filesDir()})
	case "delete":
		if err := DeleteFile(in.Name); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	case "open":
		p := filesDir()
		if in.Name != "" {
			var err error
			if p, err = FilePath(in.Name); err != nil {
				writeErr(w, err)
				return
			}
		}
		if err := openURL(p); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	case "models":
		l, err := c.Models()
		if err != nil {
			writeErr(w, err)
			return
		}
		if !contains(l, "auto") {
			l = append([]string{"auto"}, l...)
		}
		writeJSON(w, map[string]any{"models": l, "current": c.model()})
	case "model":
		m := strings.TrimSpace(in.Model)
		if m == "" {
			m = "auto"
		}
		c.SetModel(m)
		writeJSON(w, map[string]bool{"ok": true})
	case "settings":
		if r.Method == http.MethodPost && in.Set != nil {
			if err := g.saveSettings(*in.Set); err != nil {
				writeErr(w, err)
				return
			}
		}
		g.cfg.mu.RLock()
		s := settingsJSON{StartMode: g.cfg.StartMode, Autostart: !g.cfg.FreeLLMAPIManual, URL: g.cfg.FreeLLMAPI.URL,
			APIKey: g.cfg.FreeLLMAPI.APIKey, Model: g.cfg.ChatModel, System: g.cfg.ChatSystem, GUIPort: g.cfg.GUIPort}
		g.cfg.mu.RUnlock()
		writeJSON(w, s)
	case "action":
		var err error
		switch in.What {
		case "install":
			if !hasPack() {
				err = errors.New("este binario no lleva el paquete offline: descarga la versión completa del release")
			} else {
				c.Install()
			}
		case "start":
			err = c.apps.Start("freellmapi")
		case "stop":
			c.apps.Stop("freellmapi")
		case "web":
			u := g.cfg.FreeLLMAPI.Dashboard
			if u == "" {
				u = g.cfg.FreeLLMAPI.URL
			}
			err = openURL(u)
		default:
			err = errors.New("acción desconocida")
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	case "log":
		l := c.apps.Log("freellmapi", 300)
		if l == nil {
			l = []string{}
		}
		writeJSON(w, map[string]any{"lines": l})
	default:
		http.NotFound(w, r)
	}
}

func (g *GUIServer) saveSettings(s settingsJSON) error {
	if s.GUIPort < 1 || s.GUIPort > 65535 {
		return errors.New("puerto no válido")
	}
	switch s.StartMode {
	case "", "gui", "tui":
	default:
		return errors.New("modo de inicio no válido")
	}
	c := g.cfg
	c.mu.Lock()
	c.StartMode = s.StartMode
	c.FreeLLMAPIManual = !s.Autostart
	u := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(s.URL), "/"), "/v1")
	if u == "" {
		u = defaultConfig().FreeLLMAPI.URL
	}
	c.FreeLLMAPI.URL, c.FreeLLMAPI.Dashboard = u, u
	c.FreeLLMAPI.APIKey = strings.TrimSpace(s.APIKey)
	c.ChatModel = strings.TrimSpace(s.Model)
	if c.ChatModel == "" {
		c.ChatModel = "auto"
	}
	c.ChatSystem = s.System
	c.GUIPort = s.GUIPort
	c.mu.Unlock()
	return c.Save()
}

// chat responde en NDJSON: una línea por trozo de texto.
func (g *GUIServer) chat(w http.ResponseWriter, r *http.Request, text string) {
	ch, err := g.core.Chat(text)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	fl, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	gone := r.Context().Done()
	for {
		select {
		case <-gone:
			g.core.Stop()
			for range ch {
			}
			return
		case ev, ok := <-ch:
			if !ok {
				_ = enc.Encode(map[string]bool{"done": true})
				if fl != nil {
					fl.Flush()
				}
				return
			}
			o := map[string]string{}
			if ev.Delta != "" {
				o["delta"] = ev.Delta
			}
			if ev.Route != "" {
				o["route"] = ev.Route
			}
			if ev.Err != nil {
				o["error"] = ev.Err.Error()
				o["hint"] = errHint(ev.Err.Error())
			}
			if len(o) == 0 {
				continue
			}
			_ = enc.Encode(o)
			if fl != nil {
				fl.Flush()
			}
		}
	}
}
