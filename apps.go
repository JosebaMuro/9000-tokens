package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Gestor de procesos para los repos instalados desde el paquete offline.

type appProc struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	started time.Time
	exitErr error
	logPath string
}

type AppManager struct {
	cfg   *Config
	procs map[string]*appProc
	mu    sync.Mutex
}

func NewAppManager(cfg *Config) *AppManager {
	return &AppManager{cfg: cfg, procs: map[string]*appProc{}}
}

func (a *AppManager) proc(id string) *appProc {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, ok := a.procs[id]
	if !ok {
		p = &appProc{logPath: filepath.Join(dataDir(), "logs", id+".log")}
		a.procs[id] = p
	}
	return p
}

func isTermuxRuntime() bool {
	_, err := os.Stat(filepath.Join(dataDir(), "runtime", "termux", "usr", "bin", "node"))
	return err == nil
}

func nodeBin() string {
	r := filepath.Join(dataDir(), "runtime")
	if isTermuxRuntime() {
		return filepath.Join(r, "termux", "usr", "bin", "node")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(r, "node", "node.exe")
	}
	return filepath.Join(r, "node", "bin", "node")
}

func nodeInstalled() bool {
	_, err := os.Stat(nodeBin())
	return err == nil
}

func appEntry(id string) string {
	d := filepath.Join(dataDir(), "apps")
	switch id {
	case "freellmapi":
		return filepath.Join(d, "freellmapi", "server", "dist", "index.js")
	}
	return ""
}

func (a *AppManager) Installed(id string) bool {
	if !nodeInstalled() {
		return false
	}
	_, err := os.Stat(appEntry(id))
	return err == nil
}

// nodeEnv: entorno para que Node y sus hijos usen el runtime incluido.
func nodeEnv() []string {
	bin := filepath.Dir(nodeBin())
	env := os.Environ()
	sep := string(os.PathListSeparator)
	out := make([]string, 0, len(env)+8)
	for _, e := range env {
		k := strings.ToUpper(strings.SplitN(e, "=", 2)[0])
		if k == "PATH" || k == "NODE_OPTIONS" || k == "LD_LIBRARY_PATH" {
			continue
		}
		out = append(out, e)
	}
	out = append(out, "PATH="+bin+sep+os.Getenv("PATH"))
	if isTermuxRuntime() {
		usr := filepath.Join(dataDir(), "runtime", "termux", "usr")
		lib := filepath.Join(usr, "lib")
		if p := os.Getenv("LD_LIBRARY_PATH"); p != "" {
			lib += ":" + p
		}
		out = append(out, "LD_LIBRARY_PATH="+lib)
		// Node de Termux busca su openssl.cnf en /data/data/com.termux/...; si
		// Termux está instalado, esa carpeta existe pero es de otra app y
		// OpenSSL aborta con "Permission denied". Usamos la del paquete.
		cnf := filepath.Join(usr, "etc", "tls", "openssl.cnf")
		if _, err := os.Stat(cnf); err != nil {
			_ = os.MkdirAll(filepath.Dir(cnf), 0o755)
			_ = os.WriteFile(cnf, []byte("# Infinito\n"), 0o644)
		}
		out = append(out, "OPENSSL_CONF="+cnf, "SSL_CERT_DIR="+filepath.Join(usr, "etc", "tls", "certs"))
		cert := filepath.Join(usr, "etc", "tls", "cert.pem")
		if _, err := os.Stat(cert); err == nil {
			out = append(out, "SSL_CERT_FILE="+cert, "NODE_EXTRA_CA_CERTS="+cert)
		}
		if os.Getenv("TMPDIR") == "" {
			tmp := filepath.Join(dataDir(), "tmp")
			_ = os.MkdirAll(tmp, 0o755)
			out = append(out, "TMPDIR="+tmp)
		}
	}
	out = append(out, "NO_UPDATE_NOTIFIER=1", "npm_config_update_notifier=false")
	return out
}

func (a *AppManager) Running(id string) bool {
	p := a.proc(id)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil
}

func (a *AppManager) LastError(id string) error {
	p := a.proc(id)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitErr
}

func (a *AppManager) Start(id string) error {
	if !a.Installed(id) {
		return fmt.Errorf("%s no está instalado: pulsa Instalar", appName(id))
	}
	p := a.proc(id)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return nil
	}
	c := a.cfg
	var args []string
	env := nodeEnv()
	var dir string
	switch id {
	case "freellmapi":
		data := filepath.Join(dataDir(), "data", "freellmapi")
		_ = os.MkdirAll(data, 0o700)
		c.mu.Lock()
		if c.FreeLLMAPIEncKey == "" {
			c.FreeLLMAPIEncKey = randHex(32)
		}
		key := c.FreeLLMAPIEncKey
		c.mu.Unlock()
		_ = c.Save()
		args = []string{appEntry(id)}
		env = append(env,
			"NODE_ENV=production", "HOST=127.0.0.1", "PORT="+portOf(c.FreeLLMAPI.URL, "3001"),
			"ENCRYPTION_KEY="+key,
			"FREEAPI_ENV_PATH="+filepath.Join(data, ".env"),
			"FREEAPI_DB_PATH="+filepath.Join(data, "freellmapi.db"),
		)
		dir = filepath.Join(dataDir(), "apps", "freellmapi", "server")
	default:
		return fmt.Errorf("desconocido: %s", id)
	}
	_ = os.MkdirAll(filepath.Dir(p.logPath), 0o755)
	logf, err := os.OpenFile(p.logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(nodeBin(), args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logf, logf
	devnull, _ := os.Open(os.DevNull)
	cmd.Stdin = devnull
	procGroup(cmd)
	if err := cmd.Start(); err != nil {
		logf.Close()
		return err
	}
	p.cmd = cmd
	p.started = time.Now()
	p.exitErr = nil
	if id == "freellmapi" {
		go a.syncFreeLLMAPIKey(cmd)
	}
	go func() {
		err := cmd.Wait()
		logf.Close()
		p.mu.Lock()
		if p.cmd == cmd {
			p.cmd = nil
			if err != nil {
				p.exitErr = fmt.Errorf("se cerró: %v (mira el registro)", err)
			}
		}
		p.mu.Unlock()
	}()
	return nil
}

func (a *AppManager) Stop(id string) {
	p := a.proc(id)
	p.mu.Lock()
	cmd := p.cmd
	p.cmd = nil
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		killTree(cmd)
	}
}

func (a *AppManager) StopAll() {
	for _, id := range []string{"freellmapi"} {
		a.Stop(id)
	}
}

func (a *AppManager) Log(id string, n int) []string {
	b, err := os.ReadFile(a.proc(id).logPath)
	if err != nil {
		return nil
	}
	s := stripANSI(string(b))
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func stripANSI(s string) string {
	var sb strings.Builder
	esc := false
	for _, r := range s {
		if r == 0x1b {
			esc = true
			continue
		}
		if esc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				esc = false
			}
			continue
		}
		if r == '\r' {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func appName(id string) string {
	switch id {
	case "freellmapi":
		return "FreeLLMAPI"
	}
	return id
}

func portOf(url, def string) string {
	u := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(url, "/"), "/v1"), "/")
	if i := strings.LastIndex(u, ":"); i > 0 && !strings.HasSuffix(u[:i], "http") && !strings.HasSuffix(u[:i], "https") {
		return u[i+1:]
	}
	return def
}

// syncFreeLLMAPIKey copia la clave unificada que FreeLLMAPI genera en su base
// de datos a la configuración de Infinito, para que el chat pueda usarla.
func (a *AppManager) syncFreeLLMAPIKey(cmd *exec.Cmd) {
	db := filepath.Join(dataDir(), "data", "freellmapi", "freellmapi.db")
	script := `const {DatabaseSync}=require("node:sqlite");` +
		`const d=new DatabaseSync(process.argv[1],{readOnly:true});` +
		`const r=d.prepare("SELECT value FROM settings WHERE key='unified_api_key'").get();` +
		`if(r)process.stdout.write(r.value)`
	for i := 0; i < 90; i++ {
		time.Sleep(2 * time.Second)
		p := a.proc("freellmapi")
		p.mu.Lock()
		alive := p.cmd == cmd
		p.mu.Unlock()
		if !alive {
			return
		}
		if _, err := os.Stat(db); err != nil {
			continue
		}
		var out []byte
		var err error
		for _, flags := range [][]string{nil, {"--experimental-sqlite"}} {
			args := append(append(flags, "--no-warnings", "-e", script), db)
			c := exec.Command(nodeBin(), args...)
			c.Env = nodeEnv()
			if out, err = c.Output(); err == nil {
				break
			}
		}
		key := strings.TrimSpace(string(out))
		if err != nil || !strings.HasPrefix(key, "freellmapi-") {
			continue
		}
		c := a.cfg
		c.mu.Lock()
		changed := c.FreeLLMAPI.APIKey != key
		c.FreeLLMAPI.APIKey = key
		c.mu.Unlock()
		if changed {
			_ = c.Save()
		}
		return
	}
}
