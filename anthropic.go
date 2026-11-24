package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Traducción Anthropic Messages API ⇄ OpenAI Chat Completions, para que
// Claude Code pueda usar la pasarela integrada.

type aMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type aBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Source    *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
	} `json:"source,omitempty"`
}

type aReq struct {
	Model         string          `json:"model"`
	System        json.RawMessage `json:"system"`
	Messages      []aMsg          `json:"messages"`
	MaxTokens     int             `json:"max_tokens"`
	Temperature   *float64        `json:"temperature"`
	TopP          *float64        `json:"top_p"`
	StopSequences []string        `json:"stop_sequences"`
	Stream        bool            `json:"stream"`
	Tools         []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema"`
		Type        string         `json:"type"`
	} `json:"tools"`
	ToolChoice *struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"tool_choice"`
}

func parseBlocks(raw json.RawMessage) []aBlock {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []aBlock{{Type: "text", Text: s}}
	}
	var bs []aBlock
	_ = json.Unmarshal(raw, &bs)
	return bs
}

func blocksText(bs []aBlock) string {
	var parts []string
	for _, b := range bs {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func cleanSchema(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range t {
			if k == "$schema" {
				continue
			}
			out[k] = cleanSchema(val)
		}
		return out
	case []any:
		for i := range t {
			t[i] = cleanSchema(t[i])
		}
		return t
	}
	return v
}

func anthropicToOpenAI(in aReq) map[string]any {
	var msgs []map[string]any
	if sys := blocksText(parseBlocks(in.System)); sys != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": sys})
	}
	for _, m := range in.Messages {
		bs := parseBlocks(m.Content)
		if m.Role == "assistant" {
			msg := map[string]any{"role": "assistant"}
			text := blocksText(bs)
			var calls []map[string]any
			for _, b := range bs {
				if b.Type == "tool_use" {
					args := string(b.Input)
					if args == "" || args == "null" {
						args = "{}"
					}
					calls = append(calls, map[string]any{
						"id": b.ID, "type": "function",
						"function": map[string]any{"name": b.Name, "arguments": args},
					})
				}
			}
			if text != "" || len(calls) == 0 {
				msg["content"] = text
			} else {
				msg["content"] = nil
			}
			if len(calls) > 0 {
				msg["tool_calls"] = calls
			}
			msgs = append(msgs, msg)
			continue
		}
		// usuario: primero los resultados de herramientas, luego el resto.
		var parts []map[string]any
		hasImage := false
		for _, b := range bs {
			switch b.Type {
			case "tool_result":
				content := blocksText(parseBlocks(b.Content))
				if b.IsError {
					content = "ERROR: " + content
				}
				if content == "" {
					content = "(sin salida)"
				}
				msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": b.ToolUseID, "content": content})
			case "text":
				if b.Text != "" {
					parts = append(parts, map[string]any{"type": "text", "text": b.Text})
				}
			case "image":
				if b.Source != nil {
					url := b.Source.URL
					if b.Source.Type == "base64" {
						url = "data:" + b.Source.MediaType + ";base64," + b.Source.Data
					}
					parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
					hasImage = true
				}
			}
		}
		if len(parts) == 0 {
			continue
		}
		if !hasImage {
			var txt []string
			for _, p := range parts {
				txt = append(txt, p["text"].(string))
			}
			msgs = append(msgs, map[string]any{"role": "user", "content": strings.Join(txt, "\n")})
		} else {
			msgs = append(msgs, map[string]any{"role": "user", "content": parts})
		}
	}
	out := map[string]any{"model": in.Model, "messages": msgs}
	if in.MaxTokens > 0 {
		out["max_tokens"] = in.MaxTokens
	}
	if in.Temperature != nil {
		out["temperature"] = *in.Temperature
	}
	if in.TopP != nil {
		out["top_p"] = *in.TopP
	}
	if len(in.StopSequences) > 0 {
		out["stop"] = in.StopSequences
	}
	var tools []map[string]any
	for _, t := range in.Tools {
		if t.InputSchema == nil {
			continue // herramientas del servidor de Anthropic (web_search…)
		}
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": t.Name, "description": t.Description,
				"parameters": cleanSchema(t.InputSchema),
			},
		})
	}
	if len(tools) > 0 {
		out["tools"] = tools
		if tc := in.ToolChoice; tc != nil {
			switch tc.Type {
			case "any":
				out["tool_choice"] = "required"
			case "none":
				out["tool_choice"] = "none"
			case "tool":
				out["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": tc.Name}}
			}
		}
	}
	if in.Stream {
		out["stream"] = true
		out["stream_options"] = map[string]any{"include_usage": true}
	}
	return out
}

func stopReason(fr string) string {
	switch fr {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	default:
		return "end_turn"
	}
}

func msgID() string { return "msg_" + randHex(12) }

func (g *Gateway) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "invalid_request_error", "usa POST")
		return
	}
	var in aReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&in); err != nil {
		writeErr(w, 400, "invalid_request_error", "JSON no válido: "+err.Error())
		return
	}
	body := anthropicToOpenAI(in)
	model := in.Model
	if strings.HasPrefix(model, "claude-") {
		model = "auto" // los nombres de Claude se reparten por la cadena
	}
	// Algunos proveedores no aceptan stream_options: si falla se reintenta sin.
	resp, rt, err := g.dispatch(r.Context(), model, body)
	if err != nil && in.Stream {
		delete(body, "stream_options")
		resp, rt, err = g.dispatch(r.Context(), model, body)
	}
	if err != nil {
		writeErr(w, 502, "api_error", err.Error())
		return
	}
	defer resp.Body.Close()
	if in.Stream {
		g.streamAnthropic(w, resp, rt, in.Model)
		return
	}
	var oa struct {
		Choices []struct {
			Message struct {
				Content          any    `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			P int `json:"prompt_tokens"`
			C int `json:"completion_tokens"`
		} `json:"usage"`
	}
	data, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(data, &oa); err != nil || len(oa.Choices) == 0 {
		g.markFail(rt, 502, "respuesta no válida: "+short(string(data), 200), 0)
		writeErr(w, 502, "api_error", "respuesta no válida de "+rt.Key())
		return
	}
	g.markOK(rt, oa.Usage.P, oa.Usage.C)
	ch := oa.Choices[0]
	var content []map[string]any
	if s, ok := ch.Message.Content.(string); ok && s != "" {
		content = append(content, map[string]any{"type": "text", "text": s})
	}
	for _, tc := range ch.Message.ToolCalls {
		var input any = map[string]any{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		}
		id := tc.ID
		if id == "" {
			id = "toolu_" + randHex(10)
		}
		content = append(content, map[string]any{"type": "tool_use", "id": id, "name": tc.Function.Name, "input": input})
	}
	if len(content) == 0 {
		content = append(content, map[string]any{"type": "text", "text": ""})
	}
	sr := stopReason(ch.FinishReason)
	if len(ch.Message.ToolCalls) > 0 {
		sr = "tool_use"
	}
	writeJSON(w, 200, map[string]any{
		"id": msgID(), "type": "message", "role": "assistant", "model": in.Model,
		"content": content, "stop_reason": sr, "stop_sequence": nil,
		"usage": map[string]any{"input_tokens": oa.Usage.P, "output_tokens": oa.Usage.C},
	})
}

func (g *Gateway) streamAnthropic(w http.ResponseWriter, resp *http.Response, rt route, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Infinito-Route", rt.Key())
	w.WriteHeader(200)
	fl, _ := w.(http.Flusher)
	send := func(ev string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, b)
		if fl != nil {
			fl.Flush()
		}
	}
	send("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": msgID(), "type": "message", "role": "assistant", "model": model, "content": []any{},
		"stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
	}})
	send("ping", map[string]any{"type": "ping"})

	blockIdx := -1
	curType := ""               // "text" o "tool"
	toolBlocks := map[int]int{} // índice de tool_call OpenAI → índice de bloque
	closeBlock := func() {
		if blockIdx >= 0 && curType != "" {
			send("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIdx})
		}
		curType = ""
	}
	finish := ""
	inTok, outTok := 0, 0
	sawTool := false

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	lastBeat := time.Now()
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var ch struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				P int `json:"prompt_tokens"`
				C int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(payload), &ch) != nil {
			continue
		}
		if ch.Usage != nil {
			inTok, outTok = ch.Usage.P, ch.Usage.C
		}
		for _, c := range ch.Choices {
			if c.Delta.Content != "" {
				if curType != "text" {
					closeBlock()
					blockIdx++
					curType = "text"
					send("content_block_start", map[string]any{"type": "content_block_start", "index": blockIdx, "content_block": map[string]any{"type": "text", "text": ""}})
				}
				send("content_block_delta", map[string]any{"type": "content_block_delta", "index": blockIdx, "delta": map[string]any{"type": "text_delta", "text": c.Delta.Content}})
			}
			for _, tc := range c.Delta.ToolCalls {
				bi, ok := toolBlocks[tc.Index]
				if !ok {
					closeBlock()
					blockIdx++
					bi = blockIdx
					toolBlocks[tc.Index] = bi
					curType = "tool"
					sawTool = true
					id := tc.ID
					if id == "" {
						id = "toolu_" + randHex(10)
					}
					send("content_block_start", map[string]any{"type": "content_block_start", "index": bi, "content_block": map[string]any{"type": "tool_use", "id": id, "name": tc.Function.Name, "input": map[string]any{}}})
				}
				if tc.Function.Arguments != "" {
					send("content_block_delta", map[string]any{"type": "content_block_delta", "index": bi, "delta": map[string]any{"type": "input_json_delta", "partial_json": tc.Function.Arguments}})
				}
			}
			if c.FinishReason != nil && *c.FinishReason != "" {
				finish = *c.FinishReason
			}
		}
		if time.Since(lastBeat) > 10*time.Second {
			send("ping", map[string]any{"type": "ping"})
			lastBeat = time.Now()
		}
	}
	if blockIdx < 0 {
		blockIdx = 0
		send("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		curType = "text"
	}
	closeBlock()
	sr := stopReason(finish)
	if sawTool {
		sr = "tool_use"
	}
	send("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": sr, "stop_sequence": nil}, "usage": map[string]any{"input_tokens": inTok, "output_tokens": outTok}})
	send("message_stop", map[string]any{"type": "message_stop"})
	g.markOK(rt, inTok, outTok)
}
