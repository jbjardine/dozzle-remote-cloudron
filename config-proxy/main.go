package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	dataDir        = "/app/data"
	envFilePath    = "/app/data/env.sh"
	sshDir         = "/app/data/.ssh"
	sshKeyPath     = "/app/data/.ssh/id_rsa"
	sshConfigPath  = "/app/data/.ssh/config"
	reloadFlagPath = "/app/data/reload"
)

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Remote Docker Configuration</title>
  <style>
    :root { color-scheme: dark; }
    body { margin: 0; font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Arial; background: #0f1115; color: #e7e7e7; }
    .wrap { max-width: 720px; margin: 40px auto; padding: 24px; background: #161a21; border-radius: 12px; box-shadow: 0 12px 30px rgba(0,0,0,0.35); }
    h1 { font-size: 22px; margin: 0 0 16px; }
    p { color: #b7bdc9; }
    label { display: block; margin: 18px 0 8px; font-weight: 600; }
    input[type=text] { width: 100%; padding: 12px 14px; border-radius: 8px; border: 1px solid #2a3040; background: #0d1017; color: #e7e7e7; }
    input[type=password] { width: 100%; padding: 12px 14px; border-radius: 8px; border: 1px solid #2a3040; background: #0d1017; color: #e7e7e7; }
    textarea { width: 100%; min-height: 120px; padding: 12px 14px; border-radius: 8px; border: 1px solid #2a3040; background: #0d1017; color: #e7e7e7; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .hint { font-size: 12px; color: #8e96a8; margin-top: 6px; }
    .row { display: flex; gap: 12px; align-items: center; margin-top: 18px; }
    button { background: #2b6cf6; border: 0; color: #fff; padding: 10px 16px; border-radius: 8px; cursor: pointer; font-weight: 600; }
    button.secondary { background: #323a4d; }
    button.danger { background: #b53b3b; }
    .button-link { background: #323a4d; color: #fff; padding: 10px 16px; border-radius: 8px; text-decoration: none; font-weight: 600; display: inline-block; }
    .status { margin-top: 16px; padding: 10px 12px; border-radius: 8px; background: #1c2433; color: #c6d4ff; }
    .status-grid { margin-top: 18px; display: grid; gap: 10px; }
    .status-item { display: flex; justify-content: space-between; align-items: center; padding: 10px 12px; border-radius: 8px; background: #121722; border: 1px solid #202738; }
    .status-item span { color: #c8cfdb; }
    .status-ok { color: #8ddc9a; }
    .status-warn { color: #f0c36c; }
    .status-bad { color: #ff9a9a; }
    .checkline { display: flex; align-items: center; gap: 10px; margin-top: 10px; }
    code { background: #0b0f17; padding: 2px 6px; border-radius: 6px; }
  </style>
</head>
<body>
  <div class="wrap">
    <h1>Remote Docker Configuration</h1>
    <p>Set the remote Docker host used by the log viewer.</p>
    <form method="post" action="/config">
      <label for="remote_host">Remote Docker Host</label>
      <input id="remote_host" name="remote_host" type="text" value="{{ .RemoteHost }}" placeholder="tcp://10.0.0.10:2375" />
      <div class="hint">Format: <code>tcp://host:port</code> or <code>ssh://user@host[:port]</code>. Use TCP only on a private network.</div>
      <label for="remote_label">Display Name (optional)</label>
      <input id="remote_label" name="remote_label" type="text" value="{{ .RemoteLabel }}" placeholder="production-docker" />
      <div class="hint">Shown in the log viewer instead of the raw host.</div>
      <label for="ssh_key">SSH private key (optional)</label>
      <textarea id="ssh_key" name="ssh_key" placeholder="{{ if .SSHKeyConfigured }}Saved key (leave empty to keep){{ else }}-----BEGIN OPENSSH PRIVATE KEY-----{{ end }}"></textarea>
      <div class="hint">Used only for <code>ssh://</code>. Stored in <code>/app/data/.ssh/id_rsa</code>.</div>
      <div class="checkline">
        <input id="clear_ssh_key" name="clear_ssh_key" type="checkbox" />
        <label for="clear_ssh_key" style="margin:0;">Remove saved key</label>
      </div>
      <div class="row">
        <button type="submit">Save</button>
        <button type="submit" class="secondary" name="apply" value="1">Save & Apply</button>
        <a class="button-link" href="/">Open Logs</a>
      </div>
    </form>
    <form method="post" action="/config/test">
      <div class="row">
        <button type="submit" class="secondary">Test Connection</button>
      </div>
    </form>
    <div class="status-grid">
      {{ range .StatusLines }}
        <div class="status-item">
          <span>{{ .Label }}</span>
          <strong class="{{ .Class }}">{{ .Value }}</strong>
        </div>
      {{ end }}
    </div>
    {{ if .Message }}
      <div class="status">{{ .Message }}</div>
    {{ end }}
  </div>
</body>
</html>`))

type pageData struct {
	RemoteHost       string
	RemoteLabel      string
	SSHKeyConfigured bool
	Message          string
	StatusLines      []statusLine
}

type statusLine struct {
	Label string
	Value string
	Class string
}

func main() {
	target, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		log.Fatalf("invalid proxy target: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = 50 * time.Millisecond

	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/config") {
			handleConfigRoutes(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}

	http.HandleFunc("/", handler)
	log.Printf("config proxy listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func handleConfigRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/config":
		handleConfig(w, r)
	case "/config/test":
		handleTest(w, r)
	default:
		handleConfig(w, r)
	}
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := loadConfig()
		renderPage(w, cfg, "")
	case http.MethodPost:
		cfg := loadConfig()
		if err := r.ParseForm(); err != nil {
			renderPage(w, cfg, "Error: invalid form")
			return
		}
		remote := strings.TrimSpace(r.FormValue("remote_host"))
		label := strings.TrimSpace(r.FormValue("remote_label"))
		if strings.Contains(remote, "|") && label == "" {
			base, splitLabel := splitRemoteLabel(remote)
			remote = base
			label = splitLabel
		}
		sshKey := normalizeMultilineInput(r.FormValue("ssh_key"))
		clearKey := r.FormValue("clear_ssh_key") != ""
		if err := validateRemoteHost(remote); err != nil {
			renderPage(w, cfg, "Error: "+err.Error())
			return
		}
		if err := validateRemoteLabel(label); err != nil {
			renderPage(w, cfg, "Error: "+err.Error())
			return
		}
		if err := writeEnv(remote, label); err != nil {
			renderPage(w, cfg, "Error: "+err.Error())
			return
		}
		if err := writeSSHFiles(sshKey, clearKey); err != nil {
			renderPage(w, cfg, "Error: "+err.Error())
			return
		}
		updated := loadConfig()
		if r.FormValue("apply") != "" {
			if err := touchReloadFlag(); err != nil {
				renderPage(w, updated, "Error: "+err.Error())
				return
			}
			renderPage(w, updated, "Applied. Give it a few seconds to reconnect.")
			return
		}
		msg := "Configuration saved. Use “Save & Apply” to reload without restarting."
		renderPage(w, updated, msg)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg := loadConfig()
	statusLines := buildStatusLines(cfg)
	if err := validateRemoteHost(cfg.RemoteHost); err != nil {
		renderPageWithStatus(w, cfg, "Error: "+err.Error(), statusLines)
		return
	}
	if cfg.RemoteHost == "" {
		renderPageWithStatus(w, cfg, "Remote not configured", statusLines)
		return
	}
	msg := testRemoteHost(cfg.RemoteHost)
	renderPageWithStatus(w, cfg, msg, statusLines)
}

func touchReloadFlag() error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(reloadFlagPath, []byte(time.Now().Format(time.RFC3339)), 0o600)
}

func renderPage(w http.ResponseWriter, cfg config, message string) {
	renderPageWithStatus(w, cfg, message, buildStatusLines(cfg))
}

func renderPageWithStatus(w http.ResponseWriter, cfg config, message string, statusLines []statusLine) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := pageData{
		RemoteHost:       cfg.RemoteHost,
		RemoteLabel:      cfg.RemoteLabel,
		SSHKeyConfigured: cfg.SSHKeyConfigured,
		Message:          message,
		StatusLines:      statusLines,
	}
	if err := pageTmpl.Execute(w, data); err != nil {
		log.Printf("template error: %v", err)
	}
}

type config struct {
	RemoteHost       string
	RemoteLabel      string
	SSHKeyConfigured bool
}

func loadConfig() config {
	var cfg config
	content, err := os.ReadFile(envFilePath)
	if err != nil {
		return cfg
	}
	var fallback string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")
		switch key {
		case "DOZZLE_REMOTE_HOST":
			cfg.RemoteHost = val
		case "DOCKER_HOST":
			fallback = val
		case "DOZZLE_REMOTE_LABEL":
			cfg.RemoteLabel = val
		}
	}
	if cfg.RemoteHost == "" {
		cfg.RemoteHost = fallback
	}
	if cfg.RemoteLabel == "" && strings.Contains(cfg.RemoteHost, "|") {
		base, label := splitRemoteLabel(cfg.RemoteHost)
		cfg.RemoteHost = base
		cfg.RemoteLabel = label
	}
	cfg.SSHKeyConfigured = fileHasContent(sshKeyPath)
	return cfg
}

func writeEnv(remoteHost, remoteLabel string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	builder := strings.Builder{}
	builder.WriteString("# Autogenerated by Remote Log Proxy config\n")
	if remoteHost != "" {
		escaped := escapeValue(remoteHost)
		builder.WriteString("export DOZZLE_REMOTE_HOST=\"")
		builder.WriteString(escaped)
		builder.WriteString("\"\n")
	}
	if remoteLabel != "" {
		escaped := escapeValue(remoteLabel)
		builder.WriteString("export DOZZLE_REMOTE_LABEL=\"")
		builder.WriteString(escaped)
		builder.WriteString("\"\n")
	}
	builder.WriteString("# You can add other variables here\n")
	return os.WriteFile(filepath.Clean(envFilePath), []byte(builder.String()), 0o600)
}

func writeSSHFiles(sshKey string, clearKey bool) error {
	if !clearKey && sshKey == "" {
		return nil
	}
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return err
	}
	if clearKey {
		_ = os.Remove(sshKeyPath)
	} else if sshKey != "" {
		content := ensureTrailingNewline(sshKey)
		if err := os.WriteFile(sshKeyPath, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return ensureSSHConfig()
}

func ensureSSHConfig() error {
	if !fileHasContent(sshKeyPath) {
		_ = os.Remove(sshConfigPath)
		return nil
	}
	_ = os.Remove(sshConfigPath)
	return nil
}

func ensureTrailingNewline(value string) string {
	if value == "" {
		return value
	}
	if !strings.HasSuffix(value, "\n") {
		return value + "\n"
	}
	return value
}

func normalizeMultilineInput(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.TrimSpace(value)
}

func fileHasContent(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > 0
}

func escapeValue(val string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return replacer.Replace(val)
}

func validateRemoteHost(value string) error {
	if value == "" {
		return nil
	}
	base := value
	if strings.Contains(value, "|") {
		base, _ = splitRemoteLabel(value)
		if strings.HasSuffix(value, "|") {
			return errors.New("label cannot be empty")
		}
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return errors.New("invalid format")
	}
	if parsed.Scheme != "tcp" && parsed.Scheme != "ssh" {
		return errors.New("invalid scheme (tcp:// or ssh://)")
	}
	if parsed.Host == "" {
		return errors.New("missing host")
	}
	if parsed.Scheme == "tcp" {
		if _, _, err := net.SplitHostPort(parsed.Host); err != nil {
			return errors.New("tcp requires host:port")
		}
		return nil
	}
	if parsed.Scheme == "ssh" {
		host := parsed.Host
		if strings.Contains(host, ":") {
			if _, _, err := net.SplitHostPort(host); err != nil {
				return errors.New("invalid ssh port")
			}
		}
		return nil
	}
	return errors.New("invalid format")
}

func validateRemoteLabel(value string) error {
	if value == "" {
		return nil
	}
	if strings.Contains(value, "|") {
		return errors.New("label cannot contain '|'")
	}
	return nil
}

func buildStatusLines(cfg config) []statusLine {
	backendStatus := dozzleStatus()
	lines := []statusLine{
		{
			Label: "Viewer Backend",
			Value: backendStatus,
			Class: statusClass(backendStatus),
		},
	}
	remoteValue, remoteClass := remoteStatus(cfg.RemoteHost)
	lines = append(lines, statusLine{
		Label: "Remote Docker",
		Value: remoteValue,
		Class: remoteClass,
	})
	return lines
}

func statusClass(value string) string {
	switch {
	case strings.HasPrefix(value, "OK"):
		return "status-ok"
	case strings.HasPrefix(value, "WARN"):
		return "status-warn"
	default:
		return "status-bad"
	}
}

func dozzleStatus() string {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8081/")
	if err != nil {
		return "ERR - unavailable"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		return "OK - backend active"
	}
	return "WARN - status " + strconv.Itoa(resp.StatusCode)
}

func remoteStatus(remoteHost string) (string, string) {
	if remoteHost == "" {
		return "not configured", "status-warn"
	}
	base, _ := splitRemoteLabel(remoteHost)
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "invalid format", "status-bad"
	}
	switch parsed.Scheme {
	case "tcp":
		start := time.Now()
		if err := dockerPing(parsed.Host, 2*time.Second); err != nil {
			return "ERR - " + err.Error(), "status-bad"
		}
		return fmt.Sprintf("OK - %dms", time.Since(start).Milliseconds()), "status-ok"
	case "ssh":
		start := time.Now()
		if err := dockerPing("127.0.0.1:2375", 2*time.Second); err != nil {
			return "ERR - tunnel " + err.Error(), "status-bad"
		}
		return fmt.Sprintf("OK - tunnel %dms", time.Since(start).Milliseconds()), "status-ok"
	default:
		return "invalid format", "status-bad"
	}
}

func testRemoteHost(remoteHost string) string {
	base, _ := splitRemoteLabel(remoteHost)
	parsed, err := url.Parse(base)
	if err != nil {
		return "Error: invalid format"
	}
	switch parsed.Scheme {
	case "tcp":
		if _, _, err := net.SplitHostPort(parsed.Host); err != nil {
			return "Error: tcp requires host:port"
		}
		start := time.Now()
		if err := dockerPing(parsed.Host, 4*time.Second); err != nil {
			return "Test Docker: ERR - " + err.Error()
		}
		return fmt.Sprintf("Test Docker: OK (%dms)", time.Since(start).Milliseconds())
	case "ssh":
		start := time.Now()
		if err := dockerPing("127.0.0.1:2375", 4*time.Second); err != nil {
			return "Test Docker (SSH tunnel): ERR - " + err.Error()
		}
		return fmt.Sprintf("Test Docker (SSH tunnel): OK (%dms)", time.Since(start).Milliseconds())
	default:
		return "Error: invalid scheme"
	}
}

func dockerPing(host string, timeout time.Duration) error {
	client := http.Client{Timeout: timeout}
	endpoint := "http://" + host + "/_ping"
	resp, err := client.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func splitRemoteLabel(value string) (string, string) {
	parts := strings.SplitN(value, "|", 2)
	base := strings.TrimSpace(parts[0])
	label := ""
	if len(parts) == 2 {
		label = strings.TrimSpace(parts[1])
	}
	return base, label
}
