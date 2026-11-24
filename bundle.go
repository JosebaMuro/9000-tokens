package main

import (
	"archive/tar"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
)

// El paquete offline (Node + OmniRoute + FreeLLMAPI) va pegado al final del
// propio ejecutable: [binario][tar.zst][16 bytes: "INFPACK1" + offset].

const packMagic = "INFPACK1"

type packInfo struct {
	Target   string            `json:"target"`
	Built    string            `json:"built"`
	Versions map[string]string `json:"versions"`
}

func dataDir() string {
	if d := os.Getenv("INFINITO_DATA"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "infinito")
		}
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".infinito")
}

// findPack devuelve el archivo y la posición donde empieza el paquete.
func findPack() (path string, offset, size int64, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", 0, 0, err
	}
	if p, e := filepath.EvalSymlinks(exe); e == nil {
		exe = p
	}
	cands := []string{exe, exe + ".pack"}
	for _, c := range cands {
		f, err := os.Open(c)
		if err != nil {
			continue
		}
		st, _ := f.Stat()
		if st.Size() < 16 {
			f.Close()
			continue
		}
		foot := make([]byte, 16)
		_, err = f.ReadAt(foot, st.Size()-16)
		f.Close()
		if err == nil && string(foot[:8]) == packMagic {
			off := int64(binary.LittleEndian.Uint64(foot[8:]))
			return c, off, st.Size() - 16 - off, nil
		}
		if c != exe {
			// .pack sin pie: el archivo entero es el paquete
			return c, 0, st.Size(), nil
		}
	}
	return "", 0, 0, errors.New("este binario no lleva el paquete offline (es la versión ligera)")
}

func hasPack() bool {
	_, _, _, err := findPack()
	return err == nil
}

func installedInfo() (*packInfo, error) {
	b, err := os.ReadFile(filepath.Join(dataDir(), "pack.json"))
	if err != nil {
		return nil, err
	}
	var p packInfo
	return &p, json.Unmarshal(b, &p)
}

type progress struct {
	done, total atomic.Int64
	file        atomic.Value
}

type countingReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// installPack descomprime el paquete en dataDir(). Solo los componentes
// pedidos ("runtime", "omniroute", "freellmapi"); vacío = todos.
func installPack(pr *progress, only map[string]bool) error {
	path, off, size, err := findPack()
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	pr.total.Store(size)
	sec := io.NewSectionReader(f, off, size)
	dec, err := zstd.NewReader(countingReader{sec, &pr.done}, zstd.WithDecoderMaxWindow(1<<30), zstd.WithDecoderConcurrency(2))
	if err != nil {
		return err
	}
	defer dec.Close()
	root := dataDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	// Borra versiones anteriores de lo que se va a instalar.
	for _, comp := range []string{"runtime", "apps/omniroute", "apps/freellmapi"} {
		key := strings.TrimPrefix(comp, "apps/")
		if len(only) == 0 || only[key] {
			_ = os.RemoveAll(filepath.Join(root, filepath.FromSlash(comp)))
		}
	}
	tr := tar.NewReader(dec)
	var links [][2]string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("paquete dañado: %w", err)
		}
		name := strings.TrimPrefix(filepath.ToSlash(h.Name), "./")
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		comp := strings.SplitN(name, "/", 3)
		key := comp[0]
		if key == "apps" && len(comp) > 1 {
			key = comp[1]
		}
		if len(only) > 0 && key != "pack.json" && !only[key] {
			continue
		}
		dst := filepath.Join(root, filepath.FromSlash(name))
		pr.file.Store(name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			links = append(links, [2]string{h.Linkname, dst})
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			mode := fs.FileMode(h.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o200)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
	for _, l := range links {
		_ = os.MkdirAll(filepath.Dir(l[1]), 0o755)
		_ = os.Remove(l[1])
		if err := os.Symlink(l[0], l[1]); err != nil {
			// Sin permiso para enlaces (Windows): copiamos el destino.
			src := filepath.Join(filepath.Dir(l[1]), filepath.FromSlash(l[0]))
			_ = copyFile(src, l[1])
		}
	}
	pr.done.Store(size)
	postInstall(only)
	return nil
}

// postInstall: en Termux, OmniRoute ajusta sus módulos nativos en el propio
// dispositivo (sin internet; si algo falla, usa sus alternativas en JS).
func postInstall(only map[string]bool) {
	if !isTermuxRuntime() || (len(only) > 0 && !only["omniroute"]) {
		return
	}
	dir := filepath.Join(dataDir(), "apps", "omniroute", "node_modules", "omniroute")
	script := filepath.Join(dir, "scripts", "build", "postinstall.mjs")
	if _, err := os.Stat(script); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, nodeBin(), script)
	cmd.Dir = dir
	cmd.Env = append(nodeEnv(), "npm_config_offline=true")
	logPath := filepath.Join(dataDir(), "logs", "postinstall.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	if f, err := os.Create(logPath); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close()
	}
	_ = cmd.Run()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, _ := in.Stat()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// runPack es la orden interna usada al compilar el release:
//
//	infinito pack <carpeta> <binario> <salida>
func runPack(args []string) error {
	if len(args) != 3 {
		return errors.New("uso: infinito pack <carpeta> <binario> <salida>")
	}
	dir, exe, outPath := args[0], args[1], args[2]
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	in, err := os.Open(exe)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, in)
	in.Close()
	if err != nil {
		return err
	}
	enc, err := zstd.NewWriter(out, zstd.WithEncoderLevel(zstd.SpeedBestCompression), zstd.WithWindowSize(64<<20), zstd.WithEncoderConcurrency(runtime.NumCPU()))
	if err != nil {
		return err
	}
	tw := tar.NewWriter(enc)
	files := 0
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			link, _ = os.Readlink(p)
			link = filepath.ToSlash(link)
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = rel
		if info.IsDir() {
			h.Name += "/"
		}
		h.Uid, h.Gid, h.Uname, h.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			if err != nil {
				return err
			}
			files++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	foot := make([]byte, 16)
	copy(foot, packMagic)
	binary.LittleEndian.PutUint64(foot[8:], uint64(n))
	if _, err := out.Write(foot); err != nil {
		return err
	}
	st, _ := out.Stat()
	fmt.Printf("Paquete creado: %s (%d archivos, %.1f MB)\n", outPath, files, float64(st.Size())/1e6)
	return nil
}
