package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// This launcher runs the official 3x-ui panel (github.com/MHSanaei/3x-ui)
// behind a small path-based reverse proxy, so everything (admin panel,
// subscription server and VLESS WebSocket traffic) can share the single
// public port that Back4app Containers exposes (the PORT env var).
//
// 3x-ui internally runs THREE servers on three internal ports:
//   - web panel        (XUI_PORT,   default 20530)
//   - subscription     (SUB_PORT,   default 2096)  -> serves /sub/ /json/ /clash/
//   - xray VLESS+WS    (VLESS_PORT, default 20868)
// The proxy multiplexes them by HTTP path on the one public port.
//
// On first boot it also configures the panel via the official CLI so there is
// no default admin/admin credential, and it keeps the admin panel off the
// root path.
//
// NOTE: the free tier only gives ~256 MB RAM. To stay within that, the launcher
// limits Go's memory footprint and the panel runs without the bundled fail2ban
// (which needs extra privileges).

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// normalizePrefix ensures a leading and trailing slash: "panel" -> "/panel/".
func normalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// newProxy builds a reverse proxy to an internal target that keeps the
// original public Host header, so 3x-ui generates subscription links that
// point at the real domain instead of 127.0.0.1.
func newProxy(target *url.URL) *httputil.ReverseProxy {
	p := httputil.NewSingleHostReverseProxy(target)
	orig := p.Director
	p.Director = func(req *http.Request) {
		origHost := req.Host
		orig(req)
		req.Host = origHost
	}
	return p
}

func main() {
	installDir := envOr("XUI_DIR", "/app/x-ui")
	binPath := filepath.Join(installDir, "x-ui")

	publicPort := envOr("PORT", "3000") // the single port Back4app exposes
	panelPort := envOr("XUI_PORT", "20530")
	subPort := envOr("SUB_PORT", "2096")
	vlessPort := envOr("VLESS_PORT", "20868")
	wsPrefix := normalizePrefix(envOr("WS_PREFIX", "/ws/"))
	adminPath := normalizePrefix(envOr("ADMIN_PATH", "/panel/"))

	// Paths served by the subscription server (3x-ui defaults).
	subPrefixes := strings.FieldsFunc(envOr("SUB_PREFIXES", "/sub/ /json/ /clash/ /assets/"),
		func(r rune) bool { return r == ',' || r == ' ' })

	dataDir := envOr("XUI_DB_FOLDER", "/data")
	logDir := envOr("XUI_LOG_FOLDER", filepath.Join(dataDir, "logs"))
	_ = os.MkdirAll(dataDir, 0o755)
	_ = os.MkdirAll(logDir, 0o755)

	username := envOr("USERNAME", "admin")
	password := envOr("PASSWORD", "")
	if password == "" {
		password = randomToken(16)
	}

	// First boot only: set credentials, panel port and hidden base path via the
	// official CLI (avoids the default admin/admin credential). If the database
	// was wiped (ephemeral free tier), this runs again and restores the same
	// credentials from the persistent environment variables.
	dbPath := filepath.Join(dataDir, "x-ui.db")
	if !fileExists(dbPath) {
		firstBoot(binPath, installDir, dataDir, logDir, panelPort, adminPath, username, password)
	}

	cmd := exec.Command(binPath, "run")
	cmd.Dir = installDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"XUI_DB_FOLDER="+dataDir,
		"XUI_LOG_FOLDER="+logDir,
		"XUI_PORT="+panelPort,
		// Keep the panel from launching fail2ban/iptables (no NET_ADMIN on the
		// free tier, and it would just log warnings).
		"XUI_ENABLE_FAIL2BAN=false",
	)
	if err := cmd.Start(); err != nil {
		fmt.Println("failed to start x-ui:", err)
		os.Exit(1)
	}

	go startProxy(publicPort, panelPort, subPort, vlessPort, wsPrefix, adminPath, subPrefixes)

	banner := strings.Repeat("-", 56)
	fmt.Println(banner)
	fmt.Println("3x-ui is up. Public endpoints (replace <domain> with your app URL):")
	fmt.Printf("  Panel        : https://<domain>%s\n", adminPath)
	fmt.Printf("  Subscription : https://<domain>/sub/  (internal port %s)\n", subPort)
	fmt.Printf("  VLESS WS path: %s  (internal port %s)\n", wsPrefix, vlessPort)
	fmt.Printf("  Login        : %s / %s\n", username, password)
	fmt.Println("Save these credentials now - they are not printed again.")
	fmt.Println(banner)

	if err := cmd.Wait(); err != nil {
		fmt.Println("x-ui exited:", err)
		os.Exit(1)
	}
}

func firstBoot(binPath, installDir, dataDir, logDir, panelPort, adminPath, username, password string) {
	fmt.Println("First boot: configuring panel (credentials, port, base path)...")
	env := append(os.Environ(),
		"XUI_DB_FOLDER="+dataDir,
		"XUI_LOG_FOLDER="+logDir,
	)
	run := func(args ...string) {
		c := exec.Command(binPath, args...)
		c.Dir = installDir
		c.Env = env
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Printf("x-ui %v: error: %v\n", args, err)
		}
	}
	run("setting", "-username", username, "-password", password)
	run("setting", "-port", panelPort)
	run("setting", "-webBasePath", adminPath)
}

func startProxy(publicPort, panelPort, subPort, vlessPort, wsPrefix, adminPath string, subPrefixes []string) {
	time.Sleep(2 * time.Second) // let x-ui bind its ports

	panelURL, _ := url.Parse("http://127.0.0.1:" + panelPort)
	subURL, _ := url.Parse("http://127.0.0.1:" + subPort)
	vlessURL, _ := url.Parse("http://127.0.0.1:" + vlessPort)

	panelProxy := newProxy(panelURL)
	subProxy := newProxy(subURL)
	vlessProxy := newProxy(vlessURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/healthz" || path == "/":
			// Neutral response for platform health checks (reveals nothing).
			writeText(w, http.StatusOK, "ok\n")
		case strings.HasPrefix(path, wsPrefix):
			// VLESS + WebSocket traffic -> xray inbound
			vlessProxy.ServeHTTP(w, r)
		case hasAnyPrefix(path, subPrefixes):
			// Subscription links -> subscription server
			subProxy.ServeHTTP(w, r)
		case adminPath == "/" || strings.HasPrefix(path, adminPath):
			// Admin panel -> web server
			panelProxy.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	fmt.Println("Reverse proxy listening on :" + publicPort)
	if err := http.ListenAndServe(":"+publicPort, mux); err != nil {
		fmt.Println("proxy error:", err)
	}
}

func hasAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		p = normalizePrefix(p)
		if p != "/" && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func writeText(w http.ResponseWriter, code int, s string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(s))
}
