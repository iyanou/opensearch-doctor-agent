# OpenSearch Doctor Agent

[![GitHub release](https://img.shields.io/github/v/release/opensearch-doctor/agent)](https://github.com/opensearch-doctor/agent/releases/latest)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](https://go.dev/)

A lightweight, open-source diagnostic agent for OpenSearch clusters. It runs on your own infrastructure, collects health and performance data from your cluster, and sends it to the [OpenSearch Doctor](https://opensearchdoctor.com) platform for analysis, alerting, and historical trending.

**Your cluster data stays on your network.** The agent never reads your documents, never reads your queries, and never reads your passwords. See the [What it does NOT collect](#what-it-does-not-collect) section.

---

## What it is

The agent is a single Go binary that you drop onto any server running OpenSearch. It:

1. Connects to your OpenSearch cluster using read-only credentials
2. Runs a set of health checks (cluster status, node resources, shard distribution, snapshots, security config, and more)
3. Sends the results to [opensearchdoctor.com](https://opensearchdoctor.com) every 6 hours (configurable)
4. Sends a lightweight heartbeat every 5 minutes so the dashboard knows the agent is alive

The cloud platform analyses the results, calculates a health score, surfaces findings with recommendations, and fires alerts when thresholds are breached.

---

## What it collects

Every diagnostic run collects the following from your OpenSearch cluster via its REST API:

| Category | OpenSearch API | What is captured |
|---|---|---|
| **Cluster health** | `/_cluster/health` | Status (green/yellow/red), node count, active/unassigned shards, pending tasks |
| **Node stats** | `/_nodes/stats` | Node name, roles, JVM heap %, CPU %, disk %, memory %, uptime |
| **Shard distribution** | `/_cat/shards` | Unassigned count, unassigned reasons, shard count per node, average shard size |
| **Index stats** | `/_cat/indices`, `/_all/_settings` | Name, health, status, shard/replica count, doc count, store size, read-only flag |
| **Performance** | `/_nodes/stats/indices,thread_pool` | Indexing rate, search rate, search latency, thread pool rejections, segment count, query cache hit rate, fielddata evictions |
| **Snapshots** | `/_snapshot` | Repository count, last successful snapshot timestamp, failed snapshots (last 7 days) |
| **ISM policies** | `/_plugins/_ism/policies` | Policy count, indices with ISM errors, indices without a policy |
| **Security config** | `/_plugins/_security/api/*` | TLS enabled (HTTP/transport), audit logging enabled, auth backend configured, anonymous access enabled |
| **Plugins** | `/_cat/plugins` | Installed plugin names and versions |
| **Ingest pipelines** | `/_ingest/pipeline` | Pipeline count, orphaned pipelines (configured but not referenced by any index) |
| **Index templates** | `/_index_template` | Template count, overlapping patterns, unused templates |

All data is **aggregated and structural** — counts, percentages, flags. No document content, no field values, no query content is ever read.

---

## What it does NOT collect

This section matters. Before running any agent on your infrastructure, you deserve to know exactly what it does and doesn't do.

**The agent never reads:**
- ❌ Your document data (no index reads, no `_search`, no `_get`)
- ❌ Your search queries or query content
- ❌ Your OpenSearch username or password (credentials stay in `config.yaml` on your machine)
- ❌ Your OpenSearch API keys (same — local only)
- ❌ Your index mappings or field definitions
- ❌ Any data from your application (databases, files, environment variables, etc.)
- ❌ Your server hostname, IP address, or network topology beyond what OpenSearch exposes about its own nodes

**The agent never:**
- ❌ Opens any inbound ports
- ❌ Executes commands on your host OS (except during `--init` when installing a systemd service, which requires your explicit confirmation)
- ❌ Modifies your OpenSearch cluster (all API calls are read-only, except remediation commands which you trigger manually from the dashboard)

You can verify all of this by reading the source: [`internal/collector/collect.go`](internal/collector/collect.go) contains every OpenSearch API call the agent makes.

---

## Installation

### Requirements

- Linux, macOS, or Windows (x86_64 or ARM64)
- Network access to your OpenSearch cluster
- An OpenSearch user with **read-only** access (see [Minimum permissions](#minimum-permissions))
- A free account at [opensearchdoctor.com](https://opensearchdoctor.com)

### Step 1 — Get an agent key

Sign up or log in at [opensearchdoctor.com](https://opensearchdoctor.com), then go to **Settings → Agent Keys** and create a new key. Copy it — it starts with `osd_`.

### Step 2 — Download the binary

**Linux (x86_64)**
```bash
curl -Lo agent https://github.com/opensearch-doctor/agent/releases/latest/download/agent-linux-amd64
chmod +x agent
```

**Linux (ARM64)**
```bash
curl -Lo agent https://github.com/opensearch-doctor/agent/releases/latest/download/agent-linux-arm64
chmod +x agent
```

**macOS (Apple Silicon)**
```bash
curl -Lo agent https://github.com/opensearch-doctor/agent/releases/latest/download/agent-darwin-arm64
chmod +x agent
```

**macOS (Intel)**
```bash
curl -Lo agent https://github.com/opensearch-doctor/agent/releases/latest/download/agent-darwin-amd64
chmod +x agent
```

**Windows (PowerShell)**
```powershell
Invoke-WebRequest -Uri "https://github.com/opensearch-doctor/agent/releases/latest/download/agent-windows-amd64.exe" -OutFile agent.exe
```

Verify the download with SHA256 checksums published on the [Releases page](https://github.com/opensearch-doctor/agent/releases/latest).

### Step 3 — Run the setup wizard

```bash
./agent --init
```

The wizard will walk you through 5 steps:
1. Your OpenSearch endpoint (`https://localhost:9200`)
2. A display name for the cluster (shown in the dashboard)
3. OpenSearch credentials (username/password or API key)
4. TLS verification options (skip / system CA / custom CA)
5. Your OpenSearch Doctor API key (`osd_...`)

It tests both connections before writing anything, then offers to install the agent as a background service.

### Linux — systemd service (recommended)

The setup wizard handles this automatically. If you prefer manual setup:

```bash
sudo tee /etc/systemd/system/opensearch-doctor-agent.service > /dev/null <<EOF
[Unit]
Description=OpenSearch Doctor Agent
After=network.target

[Service]
Type=simple
ExecStart=/path/to/agent --config /path/to/config.yaml
Restart=always
RestartSec=30
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now opensearch-doctor-agent
```

Useful commands:
```bash
sudo systemctl status opensearch-doctor-agent
sudo journalctl -u opensearch-doctor-agent -f
sudo systemctl restart opensearch-doctor-agent
```

### macOS — launchd (optional)

```bash
sudo tee /Library/LaunchDaemons/com.opensearch-doctor.agent.plist > /dev/null <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.opensearch-doctor.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>/path/to/agent</string>
    <string>--config</string>
    <string>/path/to/config.yaml</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/var/log/opensearch-doctor-agent.log</string>
  <key>StandardErrorPath</key><string>/var/log/opensearch-doctor-agent.log</string>
</dict>
</plist>
EOF

sudo launchctl load /Library/LaunchDaemons/com.opensearch-doctor.agent.plist
```

### Windows — Task Scheduler

Run PowerShell as Administrator:

```powershell
$action  = New-ScheduledTaskAction -Execute "C:\path\to\agent.exe" -Argument '--config "C:\path\to\config.yaml"'
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 5)
Register-ScheduledTask -TaskName "OpenSearch Doctor Agent" -Action $action -Trigger $trigger -Settings $settings -RunLevel Highest
Start-ScheduledTask -TaskName "OpenSearch Doctor Agent"
```

---

## Configuration reference

Copy `config.example.yaml` to `config.yaml` and fill in your values. Every field:

```yaml
cluster:
  # Display name shown in the OpenSearch Doctor dashboard
  name: "My Production Cluster"

  # Full URL to your OpenSearch cluster (include port)
  endpoint: "https://localhost:9200"

  # Environment tag — one of: production | staging | development | custom
  environment: "production"

  # Authentication — use username+password OR api_key, not both
  username: "osd-agent"
  password: "your-password"
  # api_key: "base64-encoded-opensearch-api-key"

  # TLS — set to true for self-signed certificates
  tls_skip_verify: false

  # Path to a custom CA certificate (PEM format) — use if tls_skip_verify is false
  # and your cert is signed by a private CA
  # ca_cert_path: "/etc/ssl/certs/my-ca.pem"

saas:
  # OpenSearch Doctor API URL — do not change unless self-hosting
  api_url: "https://opensearchdoctor.com"

  # API key from Settings → Agent Keys on the dashboard
  api_key: "osd_your_key_here"

agent:
  # How often to run a full diagnostic (minutes). Default: 360 (6 hours)
  # Lower values give more granular data but use more API quota
  interval_minutes: 360

  # How often to send a heartbeat to the dashboard (seconds). Default: 300 (5 min)
  # The dashboard marks the agent offline if no heartbeat is received for 30 minutes
  heartbeat_seconds: 300

  # Log file path. Default: agent.log in the same directory as the binary
  log_file: "agent.log"

  # Run only specific check categories (leave empty to run all)
  # enabled_categories:
  #   - cluster_health
  #   - nodes
  #   - shards
  #   - indices
  #   - performance
  #   - snapshots
  #   - ism_policies
  #   - security
  #   - plugins
  #   - ingest_pipelines
  #   - templates
```

---

## CLI flags

| Flag | Description |
|---|---|
| `--init` | Run the interactive setup wizard |
| `--config <path>` | Path to config file (default: `config.yaml`) |
| `--once` | Run diagnostics once and exit — good for cron jobs or manual checks |
| `--test` | Collect data and print a summary locally; do NOT send anything to the platform |

---

## Minimum permissions

The agent only needs **read access** to your cluster. Create a dedicated user:

```json
PUT _plugins/_security/api/roles/opensearch_doctor_agent
{
  "cluster_permissions": [
    "cluster:monitor/*",
    "cluster:admin/snapshot/get",
    "cluster:admin/repository/get",
    "indices:admin/template/get",
    "indices:monitor/*"
  ],
  "index_permissions": [{
    "index_patterns": ["*"],
    "allowed_actions": [
      "indices:monitor/*",
      "indices:admin/settings/get",
      "indices:admin/mappings/get"
    ]
  }]
}

PUT _plugins/_security/api/rolesmapping/opensearch_doctor_agent
{
  "users": ["osd-agent"]
}
```

For the security diagnostics to work, the user also needs:
```json
"cluster_permissions": ["cluster:admin/opendistro/security/ssl/certs/info"]
```

---

## Building from source

Requires Go 1.22+.

```bash
git clone https://github.com/opensearch-doctor/agent
cd agent
go build -o agent ./cmd/agent

# Run tests
go test ./...

# Check for issues
go vet ./...
```

Cross-platform builds:
```bash
make build-all   # builds all 5 platform binaries into bin/
```

---

## Self-hosting

If you run your own OpenSearch Doctor backend, point the agent at it:

```yaml
saas:
  api_url: "https://your-instance.example.com"
  api_key: "osd_your_key_here"
```

Or use the environment variable (useful for containers):

```bash
OPENSEARCH_DOCTOR_URL=https://your-instance.example.com ./agent --config config.yaml
```

---

## Security notes

- All communication from the agent to the platform is over **HTTPS**
- The agent authenticates to the platform using a **bearer token** (agent key)
- The agent authenticates to OpenSearch using **username/password or API key** — these are stored only in `config.yaml` on your machine and never sent to the platform
- The `config.yaml` file is written with `600` permissions (owner read/write only)
- TLS certificate verification can be disabled for self-signed certs (`tls_skip_verify: true`) — use `ca_cert_path` for production self-signed setups
- The agent makes **only read-only** OpenSearch API calls unless you explicitly trigger a remediation command from the dashboard

---

## Contributing

Contributions are welcome. Before opening a pull request:

1. Run `go vet ./...` — must pass cleanly
2. Run `go test ./...` — must pass
3. For new diagnostic checks: add them to `internal/collector/collect.go` and document what API endpoints are called and what data is captured
4. Keep the "what it does NOT collect" contract — no document data, no query content, no credentials

Open an issue first for significant changes so we can discuss direction.

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
