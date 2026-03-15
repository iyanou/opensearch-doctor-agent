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

const saasURL = "https://app.opensearchdoctor.com"

// runInit runs the interactive setup wizard.
func runInit(configPath string) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║   OpenSearch Doctor Agent — Setup Wizard     ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// ── Step 1: OpenSearch endpoint ───────────────────────────────────────────
	fmt.Println("Step 1 of 5 — OpenSearch endpoint")
	endpoint := prompt(reader, "  Endpoint", "https://localhost:9200")
	endpoint = strings.TrimRight(endpoint, "/")
	fmt.Println()

	// ── Step 2: Cluster name ──────────────────────────────────────────────────
	fmt.Println("Step 2 of 5 — Cluster name")
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "my-cluster"
	}
	clusterName := prompt(reader, "  Cluster name", hostname)
	fmt.Println()

	// ── Step 3: OpenSearch credentials ───────────────────────────────────────
	fmt.Println("Step 3 of 5 — OpenSearch authentication")
	fmt.Println("  [1] Username + password  (default)")
	fmt.Println("  [2] API key")
	authChoice := prompt(reader, "  Choice", "1")
	fmt.Println()

	var osUsername, osPassword, osAPIKey string
	if strings.TrimSpace(authChoice) == "2" {
		osAPIKey = prompt(reader, "  OpenSearch API key", "")
	} else {
		osUsername = prompt(reader, "  Username", "admin")
		osPassword = promptSecret(reader, "  Password")
	}
	fmt.Println()

	// ── Step 4: OpenSearch Doctor API key ────────────────────────────────────
	fmt.Println("Step 4 of 5 — OpenSearch Doctor API key")
	fmt.Println("  Go to Settings → Agent Keys in your dashboard to create a key.")
	apiKey := prompt(reader, "  API key (osd_...)", "")
	if apiKey == "" {
		fmt.Println("  ✗ API key is required. Aborting.")
		os.Exit(1)
	}
	fmt.Println()

	// ── Step 5: Test connections ──────────────────────────────────────────────
	fmt.Println("Step 5 of 5 — Testing connections")

	fmt.Print("  › Connecting to OpenSearch... ")
	osVersion, err := testOpenSearch(endpoint, osUsername, osPassword, osAPIKey)
	if err != nil {
		fmt.Printf("\n  ✗ Cannot connect to OpenSearch: %v\n", err)
		fmt.Println("  Check the endpoint and credentials, then run --init again.")
		os.Exit(1)
	}
	fmt.Printf("OK (OpenSearch %s)\n", osVersion)

	fmt.Print("  › Validating API key... ")
	if err := testAPIKey(apiKey); err != nil {
		fmt.Printf("\n  ✗ API key invalid: %v\n", err)
		fmt.Println("  Generate a key at Settings → Agent Keys and try again.")
		os.Exit(1)
	}
	fmt.Println("OK")
	fmt.Println()

	// ── Write config.yaml ─────────────────────────────────────────────────────
	cfg := buildConfig(clusterName, endpoint, osUsername, osPassword, osAPIKey, apiKey)
	if err := writeConfig(configPath, cfg); err != nil {
		fmt.Printf("  ✗ Failed to write config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ Config written to %s\n\n", configPath)

	// ── Service installation ──────────────────────────────────────────────────
	fmt.Println("How do you want to run the agent?")
	fmt.Println("  [1] Install as a background service — starts automatically on boot  (recommended)")
	fmt.Println("  [2] Run once now — good for a single diagnostic snapshot")
	fmt.Println("  [3] Skip — I'll start it manually later")
	runChoice := prompt(reader, "  Choice", "1")
	fmt.Println()

	switch strings.TrimSpace(runChoice) {
	case "1":
		installService(configPath)
	case "2":
		fmt.Println("Running diagnostics now...")
		fmt.Println()
		// Return — main() will proceed with normal run
		return
	default:
		printManualInstructions(configPath)
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

func buildConfig(name, endpoint, username, password, osAPIKey, saasKey string) string {
	authBlock := ""
	if osAPIKey != "" {
		authBlock = fmt.Sprintf("  api_key: %q", osAPIKey)
	} else {
		authBlock = fmt.Sprintf("  username: %q\n  password: %q", username, password)
	}

	return fmt.Sprintf(`cluster:
  name: %q
  endpoint: %q
%s
  tls_skip_verify: true

saas:
  api_url: %q
  api_key: %q

agent:
  interval_minutes: 30
  heartbeat_seconds: 60
`, name, endpoint, authBlock, saasURL, saasKey)
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
		installLaunchd(configPath)
	case "windows":
		installTaskScheduler(configPath)
	default:
		fmt.Printf("  Unsupported OS (%s) for automatic service installation.\n", runtime.GOOS)
		printManualInstructions(configPath)
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

func installLaunchd(configPath string) {
	execPath, err := os.Executable()
	if err != nil {
		fmt.Println("  ✗ Cannot determine agent path:", err)
		return
	}
	absConfig, _ := filepath.Abs(configPath)
	absExec, _ := filepath.Abs(execPath)
	logDir := filepath.Join(os.Getenv("HOME"), "Library", "Logs", "opensearch-doctor-agent")
	os.MkdirAll(logDir, 0o755) //nolint:errcheck

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.opensearchdoctor.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>--config</string>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>%s/agent.log</string>
  <key>StandardErrorPath</key>
  <string>%s/agent.error.log</string>
</dict>
</plist>
`, absExec, absConfig, logDir, logDir)

	plistDir := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
	os.MkdirAll(plistDir, 0o755) //nolint:errcheck
	plistPath := filepath.Join(plistDir, "com.opensearchdoctor.agent.plist")

	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		fmt.Println("  ✗ Cannot write plist:", err)
		return
	}

	fmt.Println("  Loading launchd service...")
	// Unload first in case it already exists
	exec.Command("launchctl", "unload", plistPath).Run() //nolint:errcheck
	run("launchctl", "load", "-w", plistPath)

	fmt.Println()
	fmt.Println("  ✓ Service installed. The agent will start now and on every login.")
	fmt.Println("  Logs: " + logDir + "/agent.log")
	fmt.Println("  To stop:    launchctl unload " + plistPath)
	fmt.Println("  To restart: launchctl unload " + plistPath + " && launchctl load -w " + plistPath)
}

func installTaskScheduler(configPath string) {
	execPath, err := os.Executable()
	if err != nil {
		fmt.Println("  ✗ Cannot determine agent path:", err)
		return
	}
	absConfig, _ := filepath.Abs(configPath)
	absExec, _ := filepath.Abs(execPath)

	taskName := "OpenSearch Doctor Agent"
	cmd := fmt.Sprintf(`"%s" --config "%s"`, absExec, absConfig)

	fmt.Println("  Registering Task Scheduler task (may require Administrator)...")
	run("schtasks", "/create",
		"/tn", taskName,
		"/tr", cmd,
		"/sc", "onlogon",
		"/ru", os.Getenv("USERNAME"),
		"/rl", "HIGHEST",
		"/f",
	)

	// Start it immediately
	run("schtasks", "/run", "/tn", taskName)

	fmt.Println()
	fmt.Println("  ✓ Task created. The agent will start now and on every login.")
	fmt.Println("  To stop:  schtasks /end /tn \"" + taskName + "\"")
	fmt.Println("  To remove: schtasks /delete /tn \"" + taskName + "\" /f")
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
