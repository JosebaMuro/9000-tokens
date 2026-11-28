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

var version = "1.2.0"

const usage = `Infinito %s: chat de IA gratis con FreeLLMAPI, con interfaz gráfica
(GUI) y de terminal (TUI). Todo va dentro de este binario, sin internet.

Uso:
  infinito                 pregunta si abrir la GUI o la TUI
  infinito gui             abre la interfaz gráfica en el navegador
  infinito gui --no-open   sirve la interfaz gráfica sin abrir el navegador
  infinito tui             abre la interfaz de terminal
  infinito serve           solo arranca FreeLLMAPI (API en :3001), sin interfaz
  infinito install         instala Node y FreeLLMAPI desde el paquete incluido
  infinito config          muestra dónde está el archivo de configuración
  infinito version         muestra la versión

Los archivos que genera el chat (respuestas, código, imágenes, vídeo y voz)
se guardan en la carpeta Infinito de tu usuario.
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
		mode := cfg.StartMode
		if mode == "" {
			mode = chooseMode(cfg)
		}
		switch mode {
		case "gui":
			runGUI(cfg, true)
		case "tui":
			runTUI(cfg)
		}
	case "gui", "app":
		open := true
		for _, a := range args[1:] {
			if strings.TrimLeft(a, "-") == "no-open" {
				open = false
			}
		}
		runGUI(cfg, open)
	case "tui", "terminal":
		runTUI(cfg)
	case "serve", "server":
		runServe(cfg)
	case "install":
		runInstall()
	case "pack":
		if err := runPack(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
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

// onExit para FreeLLMAPI si se cierra la ventana o llega SIGTERM.
func onExit(apps *AppManager, then func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-sig
		apps.StopAll()
		then()
	}()
}

func setupTerminal() {
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
}

func runTUI(cfg *Config) {
	apps := NewAppManager(cfg)
	core := NewCore(cfg, apps)
	setupTerminal()
	p := tea.NewProgram(newModel(cfg, core), tea.WithAltScreen(), tea.WithMouseCellMotion())
	onExit(apps, func() { p.Kill() })
	_, err := p.Run()
	apps.StopAll()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runGUI(cfg *Config, open bool) {
	apps := NewAppManager(cfg)
	core := NewCore(cfg, apps)
	srv := NewGUIServer(cfg, core)
	url, err := srv.Start()
	if err != nil {
		// Probablemente ya hay otro Infinito con la GUI abierta.
		fmt.Fprintln(os.Stderr, "No pude abrir el puerto de la GUI:", err)
		if open {
			_ = openURL(cfg.guiURL() + "/")
		}
		os.Exit(1)
	}
	core.Boot()
	fmt.Printf("Infinito %s: interfaz gráfica en %s\n", version, url)
	if open {
		if err := openURL(url); err != nil {
			fmt.Println("Ábrela en tu navegador:", url)
		}
	}
	fmt.Println("Deja esta ventana abierta mientras usas Infinito. Ctrl+C para salir.")
	done := make(chan struct{})
	onExit(apps, func() { close(done) })
	<-done
	srv.Stop()
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

// runServe deja FreeLLMAPI en marcha sin interfaz (para usar su API).
func runServe(cfg *Config) {
	apps := NewAppManager(cfg)
	if !apps.Installed("freellmapi") {
		if !hasPack() {
			fmt.Fprintln(os.Stderr, "FreeLLMAPI no está instalado y este binario no lleva el paquete.")
			os.Exit(1)
		}
		fmt.Println("Instalando FreeLLMAPI…")
		if err := installPack(&progress{}, nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := apps.Start("freellmapi"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Infinito %s: FreeLLMAPI en marcha\n  API:   %s/v1\n  Panel: %s\n  Registro: %s\n",
		version, cfg.FreeLLMAPI.URL, cfg.FreeLLMAPI.Dashboard, apps.proc("freellmapi").logPath)
	for i := 0; i < 60 && cfg.FreeLLMAPI.APIKey == ""; i++ {
		time.Sleep(time.Second)
	}
	if k := cfg.FreeLLMAPI.APIKey; k != "" {
		fmt.Println("  Clave:", k)
	}
	fmt.Println("Ctrl+C para parar.")
	done := make(chan struct{})
	onExit(apps, func() { close(done) })
	<-done
}
