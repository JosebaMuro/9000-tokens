package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Backend es el servidor de FreeLLMAPI.
type Backend struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"` // sin /v1
	APIKey    string `json:"api_key"`
	Dashboard string `json:"dashboard"`
}

type Config struct {
	// GUIPort: puerto local donde se sirve la interfaz gráfica.
	GUIPort          int     `json:"gui_port"`
	FreeLLMAPI       Backend `json:"freellmapi"`
	ChatModel        string  `json:"chat_model"`
	ChatSystem       string  `json:"chat_system"`
	FreeLLMAPIEncKey string  `json:"freellmapi_encryption_key"`
	// FreeLLMAPIManual: no arrancar FreeLLMAPI solo al abrir Infinito.
	FreeLLMAPIManual bool `json:"freellmapi_manual,omitempty"`
	// StartMode: "gui", "tui" o "" (preguntar al abrir).
	StartMode string `json:"start_mode,omitempty"`

	mu   sync.RWMutex
	path string
}

func defaultConfig() *Config {
	return &Config{
		GUIPort:    8686,
		FreeLLMAPI: Backend{ID: "freellmapi", Name: "FreeLLMAPI", URL: "http://localhost:3001", Dashboard: "http://localhost:3001"},
		ChatModel:  "auto",
		ChatSystem: "Eres un asistente útil. Responde en español.",
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func configDir() string {
	if d := os.Getenv("INFINITO_HOME"); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil || d == "" {
		h, _ := os.UserHomeDir()
		d = filepath.Join(h, ".config")
	}
	return filepath.Join(d, "infinito")
}

func loadConfig() (*Config, error) {
	dir := configDir()
	path := filepath.Join(dir, "config.json")
	c := defaultConfig()
	c.path = path
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, c.Save()
		}
		return c, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return c, err
	}
	if c.GUIPort == 0 {
		c.GUIPort = 8686
	}
	if c.FreeLLMAPI.URL == "" {
		c.FreeLLMAPI = defaultConfig().FreeLLMAPI
	}
	return c, nil
}

func (c *Config) Save() error {
	c.mu.RLock()
	data, err := json.MarshalIndent(c, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func (c *Config) guiURL() string {
	return "http://127.0.0.1:" + itoa(c.GUIPort)
}
