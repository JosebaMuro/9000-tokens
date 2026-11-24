package main

import (
	"bufio"
	"context"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	// Certificados raíz de Mozilla incrustados: si el sistema (p. ej. Android
	// o un Windows sin actualizar) no ofrece los suyos, se usan estos.
	_ "golang.org/x/crypto/x509roots/fallback"
)

func itoa(i int) string { return strconv.Itoa(i) }

// En Android/Termux no existe /etc/resolv.conf, así que el resolvedor puro
// de Go intentaría usar 127.0.0.1:53 y fallaría. Aquí buscamos los DNS de
// Termux o usamos DNS públicos.
func fixDNS() {
	if runtime.GOOS == "windows" {
		return
	}
	if _, err := os.Stat("/etc/resolv.conf"); err == nil && !isTermux() {
		return
	}
	servers := readNameservers()
	if len(servers) == 0 {
		servers = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}
	}
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 4 * time.Second}
			var lastErr error
			for _, s := range servers {
				c, err := d.DialContext(ctx, network, s)
				if err == nil {
					return c, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
}

func isTermux() bool {
	if os.Getenv("TERMUX_VERSION") != "" {
		return true
	}
	p := os.Getenv("PREFIX")
	return strings.Contains(p, "com.termux")
}

func readNameservers() []string {
	cands := []string{"/etc/resolv.conf"}
	if p := os.Getenv("PREFIX"); p != "" {
		cands = append([]string{p + "/etc/resolv.conf"}, cands...)
	}
	cands = append(cands, "/data/data/com.termux/files/usr/etc/resolv.conf")
	for _, f := range cands {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		var out []string
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) >= 2 && fs[0] == "nameserver" {
				out = append(out, net.JoinHostPort(fs[1], "53"))
			}
		}
		fh.Close()
		if len(out) > 0 {
			return out
		}
	}
	return nil
}
