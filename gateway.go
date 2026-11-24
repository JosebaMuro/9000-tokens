package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// route = un modelo concreto de un proveedor concreto.
type route struct {
	P     Provider
	Model string
}

func (r route) Key() string { return r.P.ID + "/" + r.Model }

type routeStat struct {
	OK, Fail      int
	LastErr       string
	LastUsed      time.Time
	CooldownUntil time.Time
	InTok, OutTok int
}

type Gateway struct {
	cfg    *Config
	client *http.Client

	mu    sync.Mutex
	stats map[string]*routeStat
	log   []string

	srv      *http.Server
	running  bool
	addr     string
	startErr error
}

func NewGateway(cfg *Config) *Gateway {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 120 * time.Second
	return &Gateway{
		cfg:    cfg,
		client: &http.Client{Transport: tr},
		stats:  map[string]*routeStat{},
	}
}

func (g *Gateway) logf(format string, a ...any) {
	line := time.Now().Format("15:04:05") + " " + fmt.Sprintf(format, a...)
	g.mu.Lock()
	g.log = append(g.log, line)
	if len(g.log) > 300 {
		g.log = g.log[len(g.log)-300:]
	}
	g.mu.Unlock()
}

func (g *Gateway) Logs(n int) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n > len(g.log) {
		n = len(g.log)
	}
	return append([]string(nil), g.log[len(g.log)-n:]...)
}

func (g *Gateway) stat(k string) *routeStat {
	s, ok := g.stats[k]
	if !ok {
		s = &routeStat{}
		g.stats[k] = s
	}
	return s
}

type RouteInfo struct {
	Key string
	routeStat
}

func (g *Gateway) Stats() []RouteInfo {
	rs := g.chain()
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]RouteInfo, 0, len(rs))
	for _, r := range rs {
		s := g.stat(r.Key())
		out = append(out, RouteInfo{Key: r.Key(), routeStat: *s})
	}
	return out
}

// chain = cadena de respaldo: proveedores en orden y sus modelos en orden.
func (g *Gateway) chain() []route {
	var out []route
	for _, p := range g.cfg.snapshotProviders() {
		if !g.cfg.usable(p) {
			continue
		}
		for _, m := range p.Models {
			if m = strings.TrimSpace(m); m != "" {
				out = append(out, route{P: p, Model: m})
			}
		}
	}
	return out
}

// plan ordena las rutas a probar según el modelo pedido.
func (g *Gateway) plan(model string) []route {
	all := g.chain()
	model = strings.TrimSpace(model)
	var first, rest []route
	for _, r := range all {
		switch {
		case model == r.Key(), model == r.Model:
			first = append(first, r)
		case strings.HasPrefix(model, r.P.ID+"/") && model[len(r.P.ID)+1:] == r.Model:
			first = append(first, r)
		default:
			rest = append(rest, r)
		}
	}
	// "proveedor/modelo" que no está en la lista: se intenta igualmente.
	if len(first) == 0 {
		if i := strings.Index(model, "/"); i > 0 {
			pid := model[:i]
			for _, p := range g.cfg.snapshotProviders() {
				if p.ID == pid && g.cfg.usable(p) {
					first = append(first, route{P: p, Model: model[i+1:]})
				}
			}
		}
	}
	// Las que están en enfriamiento van al final.
	now := time.Now()
	g.mu.Lock()
	sort.SliceStable(rest, func(i, j int) bool {
		ci := g.stat(rest[i].Key()).CooldownUntil.After(now)
		cj := g.stat(rest[j].Key()).CooldownUntil.After(now)
		return !ci && cj
	})
	g.mu.Unlock()
	return append(first, rest...)
}

func (g *Gateway) cooling(r route) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stat(r.Key()).CooldownUntil.After(time.Now())
}

func (g *Gateway) markFail(r route, status int, msg string, retryAfter time.Duration) {
	g.mu.Lock()
	s := g.stat(r.Key())
	s.Fail++
	s.LastErr = msg
	s.LastUsed = time.Now()
	cd := time.Duration(0)
	switch {
	case status == 429:
		cd = 60 * time.Second
		if retryAfter > 0 {
			cd = retryAfter
		}
	case status == 401 || status == 403:
		cd = 10 * time.Minute
	case status == 0 || status >= 500:
		cd = 30 * time.Second
	}
	if cd > 0 {
		s.CooldownUntil = time.Now().Add(cd)
	}
	g.mu.Unlock()
	g.logf("✗ %s → %d %s", r.Key(), status, short(msg, 120))
}

func (g *Gateway) markOK(r route, in, out int) {
	g.mu.Lock()
	s := g.stat(r.Key())
	s.OK++
	s.LastUsed = time.Now()
	s.LastErr = ""
	s.InTok += in
	s.OutTok += out
	g.mu.Unlock()
	g.logf("✓ %s", r.Key())
}

func short(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "…"
	}
	return s
}

// ---------- Servidor ----------

func (g *Gateway) Start() error {
	g.mu.Lock()
	if g.running {
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()
	addr := net.JoinHostPort(g.cfg.GatewayHost, itoa(g.cfg.GatewayPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		g.mu.Lock()
		g.startErr = err
		g.mu.Unlock()
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", g.handleRoot)
	mux.HandleFunc("/health", g.handleHealth)
	mux.HandleFunc("/v1/models", g.auth(g.handleModels))
	mux.HandleFunc("/v1/chat/completions", g.auth(g.handleChat))
	mux.HandleFunc("/v1/messages", g.auth(g.handleMessages))
	mux.HandleFunc("/v1/messages/count_tokens", g.auth(g.handleCountTokens))
	// Clientes que añaden /v1 dos veces.
	mux.HandleFunc("/v1/v1/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1")
		mux.ServeHTTP(w, r)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 30 * time.Second}
	g.mu.Lock()
	g.srv = srv
	g.running = true
	g.addr = addr
	g.startErr = nil
	g.mu.Unlock()
	g.logf("Pasarela escuchando en http://%s", addr)
	go func() {
		err := srv.Serve(ln)
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			g.logf("Pasarela detenida: %v", err)
		}
	}()
	return nil
}

func (g *Gateway) Stop() {
	g.mu.Lock()
	srv := g.srv
	g.mu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	g.mu.Lock()
	g.running = false
	g.mu.Unlock()
	g.logf("Pasarela detenida")
}

func (g *Gateway) Running() (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running, g.startErr
}

func (g *Gateway) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		key := g.cfg.GatewayKey
		if key != "" {
			got := r.Header.Get("x-api-key")
			if a := r.Header.Get("Authorization"); got == "" && a != "" {
				got = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(a, "Bearer "), "bearer "))
			}
			if got != key {
				writeErr(w, 401, "authentication_error", "Clave de la pasarela incorrecta. Usa la que aparece en Ajustes del TUI.")
				return
			}
		}
		h(w, r)
	}
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, typ, msg string) {
	writeJSON(w, status, map[string]any{
		"type":  "error",
		"error": map[string]any{"type": typ, "message": msg, "code": status},
	})
}

func (g *Gateway) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeErr(w, 404, "not_found_error", "Ruta no encontrada: "+r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Infinito %s: pasarela compatible con OpenAI (/v1/chat/completions) y Anthropic (/v1/messages).\n", version)
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "routes": len(g.chain()), "version": version})
}

func (g *Gateway) handleModels(w http.ResponseWriter, r *http.Request) {
	data := []map[string]any{{"id": "auto", "object": "model", "owned_by": "infinito"}}
	for _, rt := range g.chain() {
		data = append(data, map[string]any{"id": rt.Key(), "object": "model", "owned_by": rt.P.ID})
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": data})
}

func (g *Gateway) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	writeJSON(w, 200, map[string]any{"input_tokens": len(b) / 4})
}

// upstream lanza la petición a un proveedor. Devuelve la respuesta abierta si
// es 2xx; si no, devuelve el estado y el mensaje de error.
func (g *Gateway) upstream(ctx context.Context, rt route, body map[string]any) (*http.Response, int, string, time.Duration) {
	body["model"] = rt.Model
	if c := g.cfg.MaxTokensCap; c > 0 {
		for _, k := range []string{"max_tokens", "max_completion_tokens"} {
			if v, ok := body[k].(float64); ok && int(v) > c {
				body[k] = c
			}
			if v, ok := body[k].(int); ok && v > c {
				body[k] = c
			}
		}
	}
	buf, _ := json.Marshal(body)
	url := strings.TrimRight(rt.P.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, 0, err.Error(), 0
	}
	req.Header.Set("Content-Type", "application/json")
	if rt.P.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+rt.P.APIKey)
	}
	if rt.P.ID == "openrouter" {
		req.Header.Set("X-Title", "Infinito TUI")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, 0, err.Error(), 0
	}
	if resp.StatusCode/100 == 2 {
		return resp, resp.StatusCode, "", 0
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	resp.Body.Close()
	var ra time.Duration
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			ra = time.Duration(n) * time.Second
		}
	}
	return nil, resp.StatusCode, string(msg), ra
}

// dispatch prueba la cadena hasta que un proveedor responde 2xx.
func (g *Gateway) dispatch(ctx context.Context, model string, body map[string]any) (*http.Response, route, error) {
	plan := g.plan(model)
	if len(plan) == 0 {
		return nil, route{}, errors.New("no hay proveedores configurados: abre el TUI, pestaña Proveedores, y añade al menos una clave")
	}
	var lastErr string
	tried := 0
	for i, rt := range plan {
		// Saltamos los que se están enfriando, salvo que no quede otra.
		if g.cooling(rt) && i < len(plan)-1 {
			continue
		}
		if ctx.Err() != nil {
			return nil, rt, ctx.Err()
		}
		tried++
		b := cloneMap(body)
		resp, status, msg, ra := g.upstream(ctx, rt, b)
		if resp != nil {
			return resp, rt, nil
		}
		g.markFail(rt, status, msg, ra)
		lastErr = fmt.Sprintf("%s: %d %s", rt.Key(), status, short(msg, 300))
	}
	if tried == 0 {
		return nil, route{}, errors.New("todos los proveedores están en pausa por límite de uso; espera un minuto")
	}
	return nil, route{}, errors.New("todos los proveedores fallaron. Último error: " + lastErr)
}

func cloneMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func (g *Gateway) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "invalid_request_error", "usa POST")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "invalid_request_error", "JSON no válido: "+err.Error())
		return
	}
	model, _ := body["model"].(string)
	stream, _ := body["stream"].(bool)
	resp, rt, err := g.dispatch(r.Context(), model, body)
	if err != nil {
		writeErr(w, 502, "api_error", err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("X-Infinito-Route", rt.Key())
	w.WriteHeader(resp.StatusCode)
	if stream {
		fl, _ := w.(http.Flusher)
		buf := make([]byte, 16<<10)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				if fl != nil {
					fl.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		g.markOK(rt, 0, 0)
		return
	}
	data, _ := io.ReadAll(resp.Body)
	var u struct {
		Usage struct {
			P int `json:"prompt_tokens"`
			C int `json:"completion_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(data, &u)
	g.markOK(rt, u.Usage.P, u.Usage.C)
	_, _ = w.Write(data)
}
