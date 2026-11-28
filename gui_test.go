package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCore(t *testing.T) (*Config, *Core) {
	d := t.TempDir()
	t.Setenv("INFINITO_HOME", filepath.Join(d, "c"))
	t.Setenv("INFINITO_DATA", filepath.Join(d, "d"))
	t.Setenv("INFINITO_FILES", filepath.Join(d, "f"))
	cfg, _ := loadConfig()
	return cfg, NewCore(cfg, NewAppManager(cfg))
}

func TestGUIAuth(t *testing.T) {
	cfg, core := testCore(t)
	g := &GUIServer{cfg: cfg, core: core, token: "tok"}
	h := g.auth(g.api)
	for _, c := range []struct {
		host, tok string
		want      int
	}{{"127.0.0.1:8686", "tok", 200}, {"127.0.0.1:8686", "mal", 401}, {"evil.example:8686", "tok", 401}} {
		r := httptest.NewRequest("GET", "/app/api/settings", nil)
		r.Host = c.host
		r.Header.Set("X-Token", c.tok)
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s: %d, quería %d", c.host, c.tok, w.Code, c.want)
		}
	}
}

func TestSaveCodeAndFiles(t *testing.T) {
	_, core := testCore(t)
	core.append(storedMsg{chatMsg: chatMsg{Role: "user", Content: "hola"}})
	core.append(storedMsg{chatMsg: chatMsg{Role: "assistant", Content: "Mira:\n```python\nprint(1)\n```\ny\n```html\n<p>x</p>\n```"}})
	names, err := core.SaveCode()
	if err != nil || len(names) != 2 || !strings.HasSuffix(names[0], ".py") || !strings.HasSuffix(names[1], ".html") {
		t.Fatalf("SaveCode: %v %v", names, err)
	}
	b, _ := os.ReadFile(filepath.Join(filesDir(), names[0]))
	if string(b) != "print(1)\n" {
		t.Fatalf("contenido: %q", b)
	}
	if _, err := core.ExportChat(); err != nil {
		t.Fatal(err)
	}
	if n := len(ListFiles()); n != 3 {
		t.Fatalf("archivos: %d", n)
	}
	if _, err := FilePath("../config.json"); err == nil {
		t.Fatal("FilePath acepta rutas fuera de la carpeta")
	}
	r := httptest.NewRequest("GET", "/app/files/..%2Fx?t=a", nil)
	w := httptest.NewRecorder()
	(&GUIServer{token: "a"}).serveFile(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("serveFile con ..: %d", w.Code)
	}
}
