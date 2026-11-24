package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

var httpc = &http.Client{Timeout: 0}

func v1(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func apiBase(url string, isProvider bool) string {
	if isProvider {
		return strings.TrimRight(url, "/")
	}
	return v1(url)
}

// listModels pide /models a una URL base ya completa (…/v1).
func listModels(ctx context.Context, base, key string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/models", nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("x-api-key", key)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, short(string(b), 160))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		var arr []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(b, &arr) == nil {
			for _, m := range arr {
				out.Data = append(out.Data, struct {
					ID string `json:"id"`
				}{m.ID})
			}
		} else {
			return nil, fmt.Errorf("respuesta no válida: %v", err)
		}
	}
	var ids []string
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	for _, m := range out.Models {
		id := m.ID
		if id == "" {
			id = m.Name
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatEvent struct {
	Delta string
	Route string
	Err   error
	Done  bool
}

// chatStream envía una conversación y emite los trozos por el canal.
func chatStream(ctx context.Context, base, key, model string, msgs []chatMsg) <-chan chatEvent {
	ch := make(chan chatEvent, 64)
	go func() {
		defer close(ch)
		body, _ := json.Marshal(map[string]any{"model": model, "messages": msgs, "stream": true})
		req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := httpc.Do(req)
		if err != nil {
			ch <- chatEvent{Err: err}
			return
		}
		defer resp.Body.Close()
		route := resp.Header.Get("X-Infinito-Route")
		if route == "" {
			route = resp.Header.Get("X-Omniroute-Provider")
		}
		if resp.StatusCode/100 != 2 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
			msg := string(b)
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(b, &e) == nil && e.Error.Message != "" {
				msg = e.Error.Message
			}
			ch <- chatEvent{Err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, short(msg, 400))}
			return
		}
		if route != "" {
			ch <- chatEvent{Route: route}
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "event-stream") {
			// Respuesta no streaming.
			b, _ := io.ReadAll(resp.Body)
			var r struct {
				Model   string `json:"model"`
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal(b, &r); err != nil || len(r.Choices) == 0 {
				ch <- chatEvent{Err: fmt.Errorf("respuesta no válida: %s", short(string(b), 200))}
				return
			}
			ch <- chatEvent{Delta: r.Choices[0].Message.Content}
			ch <- chatEvent{Done: true}
			return
		}
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			p := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if p == "[DONE]" {
				break
			}
			var c struct {
				Model   string `json:"model"`
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(p), &c) != nil {
				continue
			}
			if c.Error != nil {
				ch <- chatEvent{Err: fmt.Errorf("%s", c.Error.Message)}
				return
			}
			if route == "" && c.Model != "" {
				route = c.Model
				ch <- chatEvent{Route: route}
			}
			for _, x := range c.Choices {
				if x.Delta.Content != "" {
					select {
					case ch <- chatEvent{Delta: x.Delta.Content}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			ch <- chatEvent{Err: err}
			return
		}
		ch <- chatEvent{Done: true}
	}()
	return ch
}

// testProvider hace una petición mínima a un proveedor.
func testProvider(ctx context.Context, p Provider) (string, error) {
	if len(p.Models) == 0 {
		return "", fmt.Errorf("sin modelos")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	var sb strings.Builder
	for ev := range chatStream(ctx, p.BaseURL, p.APIKey, p.Models[0], []chatMsg{{Role: "user", Content: "Responde solo: OK"}}) {
		if ev.Err != nil {
			return "", ev.Err
		}
		sb.WriteString(ev.Delta)
	}
	return short(sb.String(), 60), nil
}
