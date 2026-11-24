package main

import (
	"embed"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

//go:embed guias/*.md
var guidesFS embed.FS

type guide struct {
	Title string
	Body  string
}

func loadGuides() []guide {
	ents, _ := fs.ReadDir(guidesFS, "guias")
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	var out []guide
	for _, e := range ents {
		b, _ := guidesFS.ReadFile("guias/" + e.Name())
		body := string(b)
		title := e.Name()
		if strings.HasPrefix(body, "# ") {
			nl := strings.IndexByte(body, '\n')
			title = strings.TrimSpace(body[2:nl])
			body = strings.TrimLeft(body[nl:], "\n")
		}
		out = append(out, guide{Title: title, Body: reflow(body)})
	}
	return out
}

// claudeEnv: variables para que Claude Code use una pasarela.
func claudeEnv(baseURL, key, model string) []string {
	env := []string{
		"ANTHROPIC_BASE_URL=" + strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1"),
		"ANTHROPIC_AUTH_TOKEN=" + key,
		"CLAUDE_CODE_AUTO_MODE_SERVER=0",
		"DISABLE_TELEMETRY=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}
	if model != "" {
		env = append(env, "ANTHROPIC_MODEL="+model, "ANTHROPIC_SMALL_FAST_MODEL="+model,
			"ANTHROPIC_DEFAULT_HAIKU_MODEL="+model)
	}
	return env
}

func envScript(env []string) string {
	var sb strings.Builder
	if runtime.GOOS == "windows" {
		sb.WriteString("# PowerShell\n")
		for _, e := range env {
			k, v, _ := strings.Cut(e, "=")
			fmt.Fprintf(&sb, "$env:%s = \"%s\"\n", k, v)
		}
	} else {
		for _, e := range env {
			k, v, _ := strings.Cut(e, "=")
			fmt.Fprintf(&sb, "export %s='%s'\n", k, v)
		}
	}
	sb.WriteString("claude\n")
	return sb.String()
}

func which(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// claudeCmd prepara Claude Code con las variables dadas.
func claudeCmd(env []string, args ...string) (*exec.Cmd, error) {
	bin := which("claude")
	if bin == "" {
		return nil, fmt.Errorf("no encuentro el comando 'claude'. Instálalo con: npm install -g @anthropic-ai/claude-code")
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	return cmd, nil
}

func omnirouteRunCmd(model string) (*exec.Cmd, error) {
	bin := which("omniroute")
	if bin == "" {
		return nil, fmt.Errorf("no encuentro 'omniroute'. Instálalo con: npm install -g omniroute (guía en la pestaña Guías)")
	}
	args := []string{"run", "claude"}
	if model != "" {
		args = append(args, "--model", model)
	}
	return exec.Command(bin, args...), nil
}

// startDetached arranca un programa en segundo plano (p. ej. omniroute).
func startDetached(name string, args ...string) error {
	bin := which(name)
	if bin == "" {
		return fmt.Errorf("no encuentro '%s' en el PATH", name)
	}
	cmd := exec.Command(bin, args...)
	devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func openURL(url string) error {
	// Dentro del APK: la app vigila este archivo y abre el navegador.
	if f := os.Getenv("INFINITO_OPEN_FILE"); f != "" {
		return os.WriteFile(f, []byte(url), 0o600)
	}
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	case isTermux() && which("termux-open-url") != "":
		cmd = exec.Command("termux-open-url", url)
	case which("xdg-open") != "":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("abre a mano: %s", url)
	}
	return cmd.Start()
}

// copyClipboard usa OSC52 y, si existen, las utilidades del sistema.
func copyClipboard(s string) {
	// OSC52: funciona en la mayoría de terminales, también por SSH.
	fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(s)))
	switch {
	case runtime.GOOS == "windows":
		c := exec.Command("clip")
		c.Stdin = strings.NewReader(s)
		_ = c.Run()
	case isTermux() && which("termux-clipboard-set") != "":
		c := exec.Command("termux-clipboard-set")
		c.Stdin = strings.NewReader(s)
		_ = c.Run()
	case which("wl-copy") != "":
		c := exec.Command("wl-copy")
		c.Stdin = strings.NewReader(s)
		_ = c.Run()
	case which("xclip") != "":
		c := exec.Command("xclip", "-selection", "clipboard")
		c.Stdin = strings.NewReader(s)
		_ = c.Run()
	}
}

// reflow une las líneas de cada párrafo para que el texto se ajuste al
// ancho de la pantalla (los bloques con 4 espacios se dejan tal cual).
func reflow(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		isCode := strings.HasPrefix(l, "    ")
		newItem := t == "" || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "#") ||
			(len(t) > 2 && t[0] >= '0' && t[0] <= '9' && (t[1] == '.' || (t[1] >= '0' && t[1] <= '9' && t[2] == '.')))
		if len(out) > 0 && !isCode && !newItem {
			prev := out[len(out)-1]
			pt := strings.TrimSpace(prev)
			if pt != "" && !strings.HasPrefix(prev, "    ") && !strings.HasPrefix(pt, "#") {
				out[len(out)-1] = prev + " " + t
				continue
			}
		}
		if !isCode {
			if strings.HasPrefix(t, "- ") || newItem {
				l = t
			}
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
