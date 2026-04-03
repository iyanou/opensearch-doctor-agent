package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// saasURL is the default platform URL. Override with OPENSEARCH_DOCTOR_URL env var.
var saasURL = func() string {
	if v := os.Getenv("OPENSEARCH_DOCTOR_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://opensearchdoctor.com"
}()

// runInit runs the interactive setup wizard.
func runInit(configPath string) {
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────┐")
	fmt.Println("│   OpenSearch Doctor — Agent Setup               │")
	fmt.Println("│   Answer a few questions to connect your cluster │")
	fmt.Println("└─────────────────────────────────────────────────┘")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// ── Step 1: OpenSearch endpoint ───────────────────────────────────────────
	fmt.Println("─── Step 1 of 5 ─ Your OpenSearch cluster address ──────────────────")
	fmt.Println("  This is the URL where your OpenSearch cluster is running.")
	fmt.Println("  Common examples:")
	fmt.Println("    https://localhost:9200      (local / same machine)")
	fmt.Println("    https://192.168.1.10:9200   (another server on your network)")
	fmt.Println("    https://my-server.com:9200  (remote server)")
	fmt.Println()
	endpoint := prompt(reader, "  Cluster URL", "https://localhost:9200")
	endpoint = strings.TrimRight(endpoint, "/")
	fmt.Println()

	// ── Step 2: Cluster name ──────────────────────────────────────────────────
	fmt.Println("─── Step 2 of 5 ─ Give your cluster a name ─────────────────────────")
	fmt.Println("  This name will appear in your OpenSearch Doctor dashboard.")
	fmt.Println("  Use something meaningful like: production, staging, my-server")
	fmt.Println()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "my-cluster"
	}
	clusterName := prompt(reader, "  Cluster name", hostname)
	fmt.Println()

	// ── Step 3: OpenSearch credentials ───────────────────────────────────────
	fmt.Println("─── Step 3 of 5 ─ OpenSearch login credentials ─────────────────────")
	fmt.Println("  The agent needs read access to your cluster to run diagnostics.")
	fmt.Println("  How do you connect to OpenSearch?")
	fmt.Println()
	fmt.Println("    [1] Username + password  ← most common (default)")
	fmt.Println("    [2] API key              ← if your cluster uses token-based auth")
	fmt.Println()
	authChoice := prompt(reader, "  Your choice", "1")
	fmt.Println()

	var osUsername, osPassword, osAPIKey string
	if strings.TrimSpace(authChoice) == "2" {
		fmt.Println("  Enter the OpenSearch API key (not the OpenSearch Doctor key):")
		osAPIKey = prompt(reader, "  OpenSearch API key", "")
	} else {
		fmt.Println("  Enter the username and password you use to log into OpenSearch.")
		fmt.Println("  (default admin credentials are username: admin, password: admin)")
		fmt.Println()
		osUsername = prompt(reader, "  Username", "admin")
		osPassword = promptSecret(reader, "  Password")
	}
	fmt.Println()

	// ── Step 3b: TLS verification ─────────────────────────────────────────────
	fmt.Println("─── SSL / TLS certificate verification ─────────────────────────────")
	fmt.Println("  Should the agent verify your OpenSearch server's SSL certificate?")
	fmt.Println()
	fmt.Println("    [1] Skip verification  ← easiest, works with self-signed certs (default)")
	fmt.Println("    [2] Verify using system trusted certificates  ← for publicly signed certs")
	fmt.Println("    [3] Verify using a custom CA certificate file  ← for internal/private CAs")
	fmt.Println()
	tlsChoice := prompt(reader, "  Your choice", "1")
	fmt.Println()

	tlsSkipVerify := true
	caCertPath := ""
	switch strings.TrimSpace(tlsChoice) {
	case "2":
		tlsSkipVerify = false
		fmt.Println("  The agent will verify the certificate using your system's trusted CAs.")
		fmt.Println()
	case "3":
		tlsSkipVerify = false
		fmt.Println("  Enter the full path to your CA certificate file (PEM format).")
		fmt.Println("  Example: /etc/ssl/certs/my-ca.crt  or  C:\\certs\\my-ca.crt")
		fmt.Println()
		caCertPath = prompt(reader, "  CA certificate path", "")
		fmt.Println()
	default:
		fmt.Println("  SSL verification will be skipped (recommended for most setups).")
		fmt.Println()
	}

	// ── Step 4: OpenSearch Doctor API key ────────────────────────────────────
	fmt.Println("─── Step 4 of 5 ─ Your OpenSearch Doctor API key ───────────────────")
	fmt.Println("  This key links the agent to your OpenSearch Doctor account.")
	fmt.Println()
	fmt.Println("  If you haven't created one yet:")
	fmt.Println("    1. Open your dashboard in the browser")
	fmt.Println("    2. Go to Settings → Quick Start")
	fmt.Println("    3. Type a key name and click Create")
	fmt.Println("    4. Copy the key (it starts with osd_)")
	fmt.Println()
	apiKey := prompt(reader, "  Paste your key here (osd_...)", "")
	if apiKey == "" {
		fmt.Println()
		fmt.Println("  ✗ A key is required to continue.")
		fmt.Println("    Create one at Settings → Quick Start in your dashboard, then run --init again.")
		os.Exit(1)
	}
	fmt.Println()

	// ── Step 5: Test connections ──────────────────────────────────────────────
	fmt.Println("─── Step 5 of 5 ─ Testing everything ───────────────────────────────")
	fmt.Println()

	fmt.Print("  › Connecting to OpenSearch at " + endpoint + "... ")
	osVersion, err := testOpenSearch(endpoint, osUsername, osPassword, osAPIKey)
	if err != nil {
		fmt.Println("FAILED")
		fmt.Println()
		fmt.Println("  ✗ Could not connect:", err)
		fmt.Println("  Things to check:")
		fmt.Println("    - Is the cluster URL correct?")
		fmt.Println("    - Is OpenSearch running?")
		fmt.Println("    - Are the username/password correct?")
		fmt.Println()
		fmt.Println("  Fix the issue then run --init again.")
		os.Exit(1)
	}
	fmt.Printf("OK  (OpenSearch %s)\n", osVersion)

	fmt.Print("  › Validating your OpenSearch Doctor API key... ")
	if err := testAPIKey(apiKey); err != nil {
		fmt.Println("FAILED")
		fmt.Println()
		fmt.Println("  ✗", err)
		fmt.Println("  Things to check:")
		fmt.Println("    - Did you copy the full key including the osd_ prefix?")
		fmt.Println("    - Go to Settings → Quick Start in your dashboard to create a new key.")
		fmt.Println()
		fmt.Println("  Fix the issue then run --init again.")
		os.Exit(1)
	}
	fmt.Println("OK")
	fmt.Println()

	// ── Write config.yaml ─────────────────────────────────────────────────────
	fmt.Println("  ✓ All checks passed! Writing configuration...")
	cfg := buildConfig(clusterName, endpoint, osUsername, osPassword, osAPIKey, apiKey, tlsSkipVerify, caCertPath)
	if err := writeConfig(configPath, cfg); err != nil {
		fmt.Printf("  ✗ Failed to write config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ Config saved to %s\n", configPath)
	fmt.Println()

	// ── Service installation ──────────────────────────────────────────────────
	fmt.Println("────────────────────────────────────────────────────────────────────")
	fmt.Println("  Last step — how should the agent run?")
	fmt.Println()
	fmt.Println("    [1] Start now and keep running  ← recommended")

	if runtime.GOOS == "linux" {
		fmt.Println("        Installs a systemd service — starts automatically on every boot.")
	} else {
		fmt.Println("        Starts the agent in the background right now.")
		fmt.Println("        Runs diagnostics every " + "interval_minutes" + " minutes.")
		fmt.Println("        Note: you will need to start it again after a restart.")
	}

	fmt.Println()
	fmt.Println("    [2] Run once now")
	fmt.Println("        Runs a single diagnostic check right now and exits.")
	fmt.Println("        Good for a quick test.")
	fmt.Println()
	runChoice := prompt(reader, "  Your choice", "1")
	fmt.Println()

	switch strings.TrimSpace(runChoice) {
	case "1":
		installService(configPath)
	default:
		fmt.Println("  Running diagnostics now...")
		fmt.Println()
		return
	}

	os.Exit(0)
}

// ── Connection tests ──────────────────────────────────────────────────────────

func testOpenSearch(endpoint, username, password, apiKey string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	req, err := http.NewRequest(http.MethodGet, endpoint+"/", nil)
	if err != nil {
		return "", err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+apiKey)
	} else {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d — check credentials", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		Version struct {
			Number string `json:"number"`
		} `json:"version"`
	}
	if json.Unmarshal(body, &info) == nil && info.Version.Number != "" {
		return info.Version.Number, nil
	}
	return "unknown", nil
}

func testAPIKey(apiKey string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, saasURL+"/api/agent/ping", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach %s — check your internet connection", saasURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("key not recognised — copy it again from the dashboard")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// ── Config file ───────────────────────────────────────────────────────────────

func buildConfig(name, endpoint, username, password, osAPIKey, saasKey string, tlsSkipVerify bool, caCertPath string) string {
	authBlock := ""
	if osAPIKey != "" {
		authBlock = fmt.Sprintf("  api_key: %q", osAPIKey)
	} else {
		authBlock = fmt.Sprintf("  username: %q\n  password: %q", username, password)
	}

	tlsBlock := fmt.Sprintf("  tls_skip_verify: %v", tlsSkipVerify)
	if caCertPath != "" {
		tlsBlock += fmt.Sprintf("\n  ca_cert_path: %q", caCertPath)
	}

	return fmt.Sprintf(`cluster:
  name: %q
  endpoint: %q
%s
%s

saas:
  api_key: %q

agent:
  # How often to run diagnostics (in minutes). Default: 30. Reduce for more frequent checks.
  interval_minutes: 30
  # How often to send a heartbeat to the dashboard (in seconds). Default: 300.
  heartbeat_seconds: 300
`, name, endpoint, authBlock, tlsBlock, saasKey)
}

func writeConfig(path, content string) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// ── Service installation ──────────────────────────────────────────────────────

func installService(configPath string) {
	switch runtime.GOOS {
	case "linux":
		installSystemd(configPath)
	case "darwin":
		startBackground(configPath)
	case "windows":
		startBackground(configPath)
	default:
		startBackground(configPath)
	}
}

func installSystemd(configPath string) {
	execPath, err := os.Executable()
	if err != nil {
		fmt.Println("  ✗ Cannot determine agent path:", err)
		return
	}
	absConfig, _ := filepath.Abs(configPath)
	absExec, _ := filepath.Abs(execPath)

	unitContent := fmt.Sprintf(`[Unit]
Description=OpenSearch Doctor Agent
After=network.target

[Service]
Type=simple
ExecStart=%s --config %s
Restart=always
RestartSec=30
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, absExec, absConfig)

	unitPath := "/etc/systemd/system/opensearch-doctor-agent.service"

	// Write unit file (requires sudo)
	tmpFile, err := os.CreateTemp("", "osd-agent-*.service")
	if err != nil {
		fmt.Println("  ✗ Cannot create temp file:", err)
		return
	}
	tmpFile.WriteString(unitContent)
	tmpFile.Close()

	fmt.Println("  Installing systemd service (requires sudo)...")
	run("sudo", "mv", tmpFile.Name(), unitPath)
	run("sudo", "systemctl", "daemon-reload")
	run("sudo", "systemctl", "enable", "--now", "opensearch-doctor-agent")

	fmt.Println()
	fmt.Println("  ✓ Service installed and started.")
	fmt.Println("  Useful commands:")
	fmt.Println("    sudo systemctl status opensearch-doctor-agent")
	fmt.Println("    sudo journalctl -u opensearch-doctor-agent -f")
	fmt.Println("    sudo systemctl restart opensearch-doctor-agent")
}

// startBackground launches the agent as a detached background process (macOS/Windows).
// It does NOT register any service — the user must start it again after a reboot.
func startBackground(configPath string) {
	execPath, err := os.Executable()
	if err != nil {
		fmt.Println("  ✗ Cannot determine agent path:", err)
		return
	}
	absConfig, _ := filepath.Abs(configPath)
	absExec, _ := filepath.Abs(execPath)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Use PowerShell Start-Process for reliable background launch on Windows
		cmd = exec.Command("powershell", "-Command",
			fmt.Sprintf(`Start-Process -FilePath "%s" -ArgumentList "--config","%s" -WindowStyle Hidden`, absExec, absConfig),
		)
	} else {
		// macOS / other Unix
		cmd = exec.Command(absExec, "--config", absConfig)
		cmd.SysProcAttr = newSysProcAttr() // sets Setpgid=true to detach
	}

	if err := cmd.Start(); err != nil {
		fmt.Println("  ✗ Failed to start agent in background:", err)
		fmt.Println()
		fmt.Println("  Start it manually by running:")
		fmt.Printf("    %s --config %s\n", absExec, absConfig)
		return
	}

	// Wait for the launcher to finish (the actual agent runs detached)
	cmd.Wait() //nolint:errcheck

	fmt.Println()
	fmt.Println("  ✓ Agent started in the background.")
	fmt.Println()

	if runtime.GOOS == "windows" {
		fmt.Println("  To verify it is running:")
		fmt.Println(`    tasklist | findstr agent`)
		fmt.Println()
		fmt.Println("  To stop it:")
		fmt.Println(`    taskkill /IM agent.exe /F`)
	} else {
		fmt.Println("  To verify it is running:")
		fmt.Println(`    pgrep -a agent`)
		fmt.Println()
		fmt.Println("  To stop it:")
		fmt.Println(`    pkill agent`)
	}

	fmt.Println()
	fmt.Println("  Note: the agent will NOT restart automatically after a reboot.")
	fmt.Println("  To start it again after a restart, run:")
	fmt.Printf("    %s --config %s\n", absExec, absConfig)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func prompt(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func promptSecret(reader *bufio.Reader, label string) string {
	// On most terminals we can't hide input without a dependency,
	// so just prompt normally. The value is not echoed on most CI/terminal setups.
	fmt.Printf("%s: ", label)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("  ✗ %s %s: %v\n", name, strings.Join(args, " "), err)
	}
}

func printManualInstructions(configPath string) {
	execPath, _ := os.Executable()
	fmt.Println("  To start the agent manually:")
	fmt.Printf("    %s --config %s\n\n", execPath, configPath)
	fmt.Println("  To run a single diagnostic:")
	fmt.Printf("    %s --config %s --once\n", execPath, configPath)
}
