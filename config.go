package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Provider es un proveedor con API compatible con OpenAI que la pasarela
// integrada puede usar.
type Provider struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key"`
	Models  []string `json:"models"` // en orden de preferencia
	Enabled bool     `json:"enabled"`
	NoKey   bool     `json:"no_key,omitempty"` // no necesita clave
	KeyURL  string   `json:"key_url,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// Backend es una pasarela externa (OmniRoute, FreeLLMAPI) o la integrada.
type Backend struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"` // sin /v1
	APIKey    string `json:"api_key"`
	Dashboard string `json:"dashboard"`
}

type Config struct {
	GatewayHost      string     `json:"gateway_host"`
	GatewayPort      int        `json:"gateway_port"`
	GatewayKey       string     `json:"gateway_key"`
	GatewayAutoRun   bool       `json:"gateway_autostart"`
	MaxTokensCap     int        `json:"max_tokens_cap"`
	Providers        []Provider `json:"providers"`
	OmniRoute        Backend    `json:"omniroute"`
	FreeLLMAPI       Backend    `json:"freellmapi"`
	ChatBackend      string     `json:"chat_backend"`
	ChatModel        string     `json:"chat_model"`
	ChatSystem       string     `json:"chat_system"`
	FreeLLMAPIEncKey string     `json:"freellmapi_encryption_key"`

	mu   sync.RWMutex
	path string
}

func defaultProviders() []Provider {
	return []Provider{
		{ID: "groq", Name: "Groq", BaseURL: "https://api.groq.com/openai/v1", Models: []string{"llama-3.3-70b-versatile", "openai/gpt-oss-120b", "qwen/qwen3-32b"}, KeyURL: "https://console.groq.com/keys"},
		{ID: "gemini", Name: "Google Gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Models: []string{"gemini-2.5-flash", "gemini-2.5-flash-lite"}, KeyURL: "https://aistudio.google.com/apikey"},
		{ID: "cerebras", Name: "Cerebras", BaseURL: "https://api.cerebras.ai/v1", Models: []string{"gpt-oss-120b", "qwen-3-235b-a22b-instruct-2507", "llama-3.3-70b"}, KeyURL: "https://cloud.cerebras.ai"},
		{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", Models: []string{"qwen/qwen3-coder:free", "deepseek/deepseek-chat-v3.1:free", "meta-llama/llama-3.3-70b-instruct:free"}, KeyURL: "https://openrouter.ai/keys"},
		{ID: "mistral", Name: "Mistral", BaseURL: "https://api.mistral.ai/v1", Models: []string{"mistral-large-latest", "devstral-small-latest", "codestral-latest"}, KeyURL: "https://console.mistral.ai/api-keys"},
		{ID: "nvidia", Name: "NVIDIA NIM", BaseURL: "https://integrate.api.nvidia.com/v1", Models: []string{"qwen/qwen3-coder-480b-a35b-instruct", "deepseek-ai/deepseek-v3.1", "meta/llama-3.3-70b-instruct"}, KeyURL: "https://build.nvidia.com"},
		{ID: "github", Name: "GitHub Models", BaseURL: "https://models.github.ai/inference", Models: []string{"openai/gpt-4.1", "openai/gpt-4.1-mini"}, KeyURL: "https://github.com/settings/tokens"},
		{ID: "sambanova", Name: "SambaNova", BaseURL: "https://api.sambanova.ai/v1", Models: []string{"DeepSeek-V3.1", "Meta-Llama-3.3-70B-Instruct"}, KeyURL: "https://cloud.sambanova.ai/apis"},
		{ID: "huggingface", Name: "Hugging Face", BaseURL: "https://router.huggingface.co/v1", Models: []string{"openai/gpt-oss-120b", "Qwen/Qwen3-Coder-480B-A35B-Instruct"}, KeyURL: "https://huggingface.co/settings/tokens"},
		{ID: "cohere", Name: "Cohere", BaseURL: "https://api.cohere.ai/compatibility/v1", Models: []string{"command-a-03-2025"}, KeyURL: "https://dashboard.cohere.com/api-keys"},
		{ID: "zai", Name: "Z.ai (Zhipu)", BaseURL: "https://api.z.ai/api/paas/v4", Models: []string{"glm-4.5-flash"}, KeyURL: "https://z.ai/manage-apikey/apikey-list"},
		{ID: "siliconflow", Name: "SiliconFlow", BaseURL: "https://api.siliconflow.com/v1", Models: []string{"Qwen/Qwen3-8B"}, KeyURL: "https://cloud.siliconflow.com/account/ak"},
		{ID: "ollama", Name: "Ollama Cloud", BaseURL: "https://ollama.com/v1", Models: []string{"gpt-oss:120b", "qwen3-coder:480b"}, KeyURL: "https://ollama.com/settings/keys"},
		{ID: "llm7", Name: "LLM7 (sin clave)", BaseURL: "https://api.llm7.io/v1", Models: []string{"gpt-4.1-nano"}, NoKey: true, KeyURL: "https://token.llm7.io"},
		{ID: "local", Name: "Local (Ollama/llama.cpp/LM Studio)", BaseURL: "http://127.0.0.1:11434/v1", Models: []string{"qwen2.5-coder:7b"}, NoKey: true, Note: "Cambia la URL y el modelo a tu servidor local"},
	}
}

func defaultConfig() *Config {
	return &Config{
		GatewayHost:    "127.0.0.1",
		GatewayPort:    8686,
		GatewayKey:     "inf-" + randHex(12),
		GatewayAutoRun: true,
		MaxTokensCap:   8192,
		Providers:      defaultProviders(),
		OmniRoute:      Backend{ID: "omniroute", Name: "OmniRoute", URL: "http://localhost:20128", Dashboard: "http://localhost:20128"},
		FreeLLMAPI:     Backend{ID: "freellmapi", Name: "FreeLLMAPI", URL: "http://localhost:3001", Dashboard: "http://localhost:3001"},
		ChatBackend:    "infinito",
		ChatModel:      "auto",
		ChatSystem:     "Eres un asistente útil. Responde en español.",
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
	// Añade proveedores nuevos del catálogo que no estén en la config.
	have := map[string]bool{}
	for _, p := range c.Providers {
		have[p.ID] = true
	}
	for _, p := range defaultProviders() {
		if !have[p.ID] {
			c.Providers = append(c.Providers, p)
		}
	}
	if c.GatewayPort == 0 {
		c.GatewayPort = 8686
	}
	if c.GatewayHost == "" {
		c.GatewayHost = "127.0.0.1"
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

// snapshotProviders devuelve una copia segura para la pasarela.
func (c *Config) snapshotProviders() []Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Provider, len(c.Providers))
	for i, p := range c.Providers {
		p.Models = append([]string(nil), p.Models...)
		out[i] = p
	}
	return out
}

func (c *Config) usable(p Provider) bool {
	return p.Enabled && (p.NoKey || p.APIKey != "") && p.BaseURL != "" && len(p.Models) > 0
}

func (c *Config) gatewayURL() string {
	host := c.GatewayHost
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	return "http://" + host + ":" + itoa(c.GatewayPort)
}

// Backends que el TUI puede usar como origen del chat.
func (c *Config) backends() []Backend {
	return []Backend{
		{ID: "infinito", Name: "Pasarela integrada", URL: c.gatewayURL(), APIKey: c.GatewayKey},
		c.OmniRoute,
		c.FreeLLMAPI,
	}
}

func (c *Config) backendByID(id string) Backend {
	for _, b := range c.backends() {
		if b.ID == id {
			return b
		}
	}
	return c.backends()[0]
}
