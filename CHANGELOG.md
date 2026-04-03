# Changelog

All notable changes to the OpenSearch Doctor Agent are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
This project uses [semantic versioning](https://semver.org/).

---

## [0.1.0] — 2025

### Initial release

**Diagnostic checks:**
- Cluster health (status, node count, unassigned shards, pending tasks)
- Node stats (heap %, CPU %, disk %, memory %, uptime per node)
- Shard distribution (unassigned count and reasons, shards per node, avg shard size)
- Index stats (health, status, shard/replica count, doc count, store size, read-only flag)
- Performance metrics (indexing rate, search latency, thread pool rejections, query cache hit rate, fielddata evictions, segment count)
- Snapshot health (repository count, last successful snapshot, failed snapshots in last 7 days)
- ISM policy coverage (policy count, indices without a policy, indices with ISM errors)
- Security config (TLS enabled on HTTP/transport, audit logging, anonymous access, auth backend)
- Installed plugins (names and versions)
- Ingest pipelines (count, orphaned pipelines)
- Index templates (count, overlapping priorities, unused templates)

**Agent features:**
- Interactive `--init` wizard with connection testing and service installation
- `--test` mode: collect and print results locally without sending to platform
- `--once` mode: single diagnostic run for cron/manual use
- systemd service installation on Linux
- Background process launch on macOS and Windows
- Configurable diagnostic interval and heartbeat interval
- Remediation command execution (triggered from dashboard)
- Heartbeat and cluster registration
