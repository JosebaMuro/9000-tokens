package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

var version = "1.0.0"

const usage = `Infinito %s: frontend de terminal para OmniRoute y FreeLLMAPI,
con pasarela de IA gratis integrada. Todo va dentro de este binario.

Uso:
  infinito                 abre la interfaz (TUI)
  infinito serve [omniroute] [freellmapi]
                           arranca la pasarela integrada (y los repos que
                           indiques) sin interfaz
  infinito install         instala Node, OmniRoute y FreeLLMAPI desde el
                           paquete offline incluido en el binario
  infinito claude [args]   abre Claude Code conectado a la pasarela integrada
  infinito env             muestra las variables para conectar Claude Code
  infinito config          muestra dónde está el archivo de configuración
  infinito version         muestra la versión

Pasarela integrada (por defecto):
  OpenAI:    http://127.0.0.1:8686/v1      modelo: auto
  Anthropic: http://127.0.0.1:8686
`

func main() {
	if p := os.Getenv("INFINITO_CRASH_FILE"); p != "" {
		if f, err := os.Create(p); err == nil {
			_ = debug.SetCrashOutput(f, debug.CrashOptions{})
		}
	}
	fixDNS()
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Aviso: no pude leer la configuración:", err)
	}
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd = strings.TrimLeft(args[0], "-")
	}
	switch cmd {
	case "":
		runTUI(cfg)
	case "serve", "server", "gateway":
		runServe(cfg, args[1:])
	case "install":
		runInstall()
	case "pack":
		if err := runPack(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "claude", "run":
		if cmd == "run" && len(args) > 1 && args[1] == "claude" {
			args = args[1:]
		}
		runClaude(cfg, args[1:])
	case "env":
		fmt.Print(envScript(claudeEnv(cfg.gatewayURL(), cfg.GatewayKey, "auto")))
	case "config":
		fmt.Println(cfg.path)
	case "version", "v":
		fmt.Println("infinito", version)
	case "help", "h":
		fmt.Printf(usage, version)
	default:
		fmt.Printf(usage, version)
		os.Exit(2)
	}
}

func runTUI(cfg *Config) {
	gw := NewGateway(cfg)
	if cfg.GatewayAutoRun {
		_ = gw.Start() // si el puerto está ocupado, el Panel lo indica
	}
	apps := NewAppManager(cfg)
	// Evita preguntar el color de fondo al terminal (algunos no contestan y
	// la interfaz tardaría segundos en salir). Oscuro salvo que COLORFGBG
	// diga lo contrario.
	dark := true
	if v := os.Getenv("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		if bg := parts[len(parts)-1]; bg == "7" || bg == "15" {
			dark = false
		}
	}
	lipgloss.SetHasDarkBackground(dark)
	zone.NewGlobal()
	p := tea.NewProgram(newModel(cfg, gw, apps), tea.WithAltScreen(), tea.WithMouseCellMotion())
	// Si se cierra la ventana del terminal, no dejamos procesos huérfanos.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-hup
		apps.StopAll()
		gw.Stop()
		p.Kill()
	}()
	_, err := p.Run()
	apps.StopAll()
	gw.Stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runInstall() {
	pr := &progress{}
	done := make(chan error, 1)
	go func() { done <- installPack(pr, nil) }()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				fmt.Fprintln(os.Stderr, "\nError:", err)
				os.Exit(1)
			}
			fmt.Println("\nInstalado en", dataDir())
			return
		case <-t.C:
			if tot := pr.total.Load(); tot > 0 {
				fmt.Printf("\rInstalando… %3.0f%%", 100*float64(pr.done.Load())/float64(tot))
			}
		}
	}
}

func runServe(cfg *Config, which []string) {
	gw := NewGateway(cfg)
	apps := NewAppManager(cfg)
	for _, id := range which {
		if err := apps.Start(strings.ToLower(id)); err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Printf("%s arrancado (registro en %s)\n", appName(id), apps.proc(id).logPath)
		}
	}
	defer apps.StopAll()
	if err := gw.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "No pude abrir el puerto:", err)
		os.Exit(1)
	}
	fmt.Printf("Infinito %s: pasarela en marcha\n", version)
	fmt.Printf("  OpenAI:    %s/v1\n  Anthropic: %s\n  Clave:     %s\n  Rutas:     %d\n", cfg.gatewayURL(), cfg.gatewayURL(), cfg.GatewayKey, len(gw.chain()))
	if len(gw.chain()) == 0 {
		fmt.Println("  Aviso: no hay proveedores con clave. Ejecuta 'infinito' y añade claves en Proveedores.")
	}
	fmt.Println("Ctrl+C para parar.")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	seen := 0
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-sig:
			gw.Stop()
			return
		case <-tick.C:
			lines := gw.Logs(300)
			if len(lines) < seen {
				seen = 0
			}
			for _, l := range lines[seen:] {
				fmt.Println(l)
			}
			seen = len(lines)
		}
	}
}

func runClaude(cfg *Config, extra []string) {
	gw := NewGateway(cfg)
	if err := gw.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "Aviso: el puerto ya está en uso, uso la pasarela que ya está en marcha:", err)
	}
	cmd, err := claudeCmd(claudeEnv(cfg.gatewayURL(), cfg.GatewayKey, "auto"), extra...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt)
	err = cmd.Run()
	gw.Stop()
	if err != nil {
		os.Exit(1)
	}
}
