# OpenSearch Doctor Agent

A lightweight agent that runs on your infrastructure, collects diagnostic data from your OpenSearch cluster, and sends it to the [OpenSearch Doctor](https://opensearchdoctor.com) platform for analysis.

**Your cluster credentials never leave your network.** The agent only sends diagnostic metrics and health data — not your index data.

---

## Quick start

### 1. Create an agent key
Go to **Settings → Agent Keys** in your OpenSearch Doctor dashboard and create a new key.

### 2. Download the agent

| Platform | Download |
|---|---|
| Linux x86_64 | [agent-linux-amd64](https://github.com/iyanou/opensearch-doctor-agent/releases/latest/download/agent-linux-amd64) |
| Linux ARM64 | [agent-linux-arm64](https://github.com/iyanou/opensearch-doctor-agent/releases/latest/download/agent-linux-arm64) |
| macOS (Apple Silicon) | [agent-darwin-arm64](https://github.com/iyanou/opensearch-doctor-agent/releases/latest/download/agent-darwin-arm64) |
| macOS (Intel) | [agent-darwin-amd64](https://github.com/iyanou/opensearch-doctor-agent/releases/latest/download/agent-darwin-amd64) |
| Windows | [agent-windows-amd64.exe](https://github.com/iyanou/opensearch-doctor-agent/releases/latest/download/agent-windows-amd64.exe) |

### 3. Run the setup wizard

```bash
chmod +x agent-linux-amd64
./agent-linux-amd64 --init
```

The wizard will:
- Ask for your OpenSearch endpoint and credentials
- Ask for your OpenSearch Doctor API key
- Test the connection to your cluster
- Write `config.yaml`
- Optionally install the agent as a background service (systemd / launchd / Task Scheduler)

---

## Manual configuration

Copy `config.example.yaml` to `config.yaml` and fill in your values:

```yaml
cluster:
  name: "my-cluster"
  endpoint: "https://my-opensearch:9200"
  username: "admin"
  password: "..."
  tls_skip_verify: true   # set to false if you have a valid TLS cert

saas:
  api_url: "https://app.opensearchdoctor.com"
  api_key: "osd_..."      # from Settings → Agent Keys

agent:
  interval_minutes: 30    # how often to run diagnostics
  heartbeat_seconds: 60   # heartbeat interval
```

Then run:
```bash
./agent --config config.yaml
```

---

## Flags

| Flag | Description |
|---|---|
| `--init` | Interactive setup wizard |
| `--config <path>` | Path to config file (default: `config.yaml`) |
| `--once` | Run diagnostics once and exit |
| `--test` | Run diagnostics, print results locally, do NOT send to platform |

---

## Building from source

Requires Go 1.22+.

```bash
git clone https://github.com/iyanou/opensearch-doctor-agent
cd opensearch-doctor-agent
go build -o agent ./cmd/agent
```

---

## Security

- The agent authenticates to OpenSearch Doctor using a bearer token (agent key)
- The agent authenticates to OpenSearch using username/password or API key
- All communication to the SaaS platform is over HTTPS
- Cluster credentials are stored only in your local `config.yaml` — never sent to the platform
- TLS certificate verification can be disabled for self-signed certificates (`tls_skip_verify: true`)

---

## License

MIT
