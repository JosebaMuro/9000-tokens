package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicToOpenAI(t *testing.T) {
	var in aReq
	raw := `{"model":"claude-x","max_tokens":100,"system":"sé breve",
	"tools":[{"name":"Bash","description":"d","input_schema":{"type":"object","$schema":"x"}}],
	"messages":[{"role":"user","content":"hola"},
	{"role":"assistant","content":[{"type":"text","text":"voy"},{"type":"tool_use","id":"t1","name":"Bash","input":{"c":"ls"}}]},
	{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"a.txt"},{"type":"text","text":"sigue"}]}]}`
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	out := anthropicToOpenAI(in)
	msgs := out["messages"].([]map[string]any)
	roles := []string{}
	for _, m := range msgs {
		roles = append(roles, m["role"].(string))
	}
	if got := strings.Join(roles, ","); got != "system,user,assistant,tool,user" {
		t.Fatalf("roles = %s", got)
	}
	if msgs[3]["tool_call_id"] != "t1" || msgs[3]["content"] != "a.txt" {
		t.Fatalf("tool msg = %v", msgs[3])
	}
	b, _ := json.Marshal(out["tools"])
	if strings.Contains(string(b), "$schema") {
		t.Fatal("$schema no se quitó")
	}
}

func TestPortOf(t *testing.T) {
	cases := map[string]string{
		"http://localhost:20128":   "20128",
		"http://127.0.0.1:3001/v1": "3001",
		"http://localhost":         "9",
	}
	for u, want := range cases {
		if got := portOf(u, "9"); got != want {
			t.Errorf("portOf(%q) = %q, quiero %q", u, got, want)
		}
	}
}

func TestReflow(t *testing.T) {
	in := "Una línea\ny su continuación.\n\n    codigo  sin tocar\n1. Paso uno\n   sigue el paso\n- punto"
	want := "Una línea y su continuación.\n\n    codigo  sin tocar\n1. Paso uno sigue el paso\n- punto"
	if got := reflow(in); got != want {
		t.Fatalf("reflow:\n%q\nquiero\n%q", got, want)
	}
}
