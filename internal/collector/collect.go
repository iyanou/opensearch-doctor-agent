package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	opensearch "github.com/opensearch-project/opensearch-go/v2"
	"go.uber.org/zap"
)

// Payload is the structured diagnostic data sent to the SaaS API.
type Payload struct {
	CollectedAt     time.Time        `json:"collectedAt"`
	DurationMs      int64            `json:"durationMs"`
	ClusterHealth   *ClusterHealth   `json:"clusterHealth,omitempty"`
	Nodes           *NodesData       `json:"nodes,omitempty"`
	Shards          *ShardsData      `json:"shards,omitempty"`
	Indices         *IndicesData     `json:"indices,omitempty"`
	Performance     *PerformanceData `json:"performance,omitempty"`
	Snapshots       *SnapshotsData   `json:"snapshots,omitempty"`
	IsmPolicies     *IsmPoliciesData `json:"ismPolicies,omitempty"`
	Security        *SecurityData    `json:"security,omitempty"`
	Plugins         *PluginsData     `json:"plugins,omitempty"`
	IngestPipelines *IngestData      `json:"ingestPipelines,omitempty"`
	Templates       *TemplatesData   `json:"templates,omitempty"`
}

// Collect runs all enabled diagnostic checks against the cluster.
// GetClusterUUID fetches the stable cluster_uuid from the root endpoint.
// Returns an empty string if the call fails (agent will still register without it).
func GetClusterUUID(ctx context.Context, client *opensearch.Client) string {
	body, err := get(ctx, client, "/")
	if err != nil {
		return ""
	}
	var info struct {
		ClusterUUID string `json:"cluster_uuid"`
	}
	if json.Unmarshal(body, &info) != nil {
		return ""
	}
	return info.ClusterUUID
}

func Collect(ctx context.Context, client *opensearch.Client, log *zap.Logger) (*Payload, error) {
	start := time.Now()
	payload := &Payload{CollectedAt: start}

	health, err := collectClusterHealth(ctx, client)
	if err != nil {
		log.Warn("cluster health collection failed", zap.Error(err))
	} else {
		payload.ClusterHealth = health
		log.Info("cluster health collected", zap.String("status", health.Status))
	}

	nodes, err := collectNodes(ctx, client)
	if err != nil {
		log.Warn("nodes collection failed", zap.Error(err))
	} else {
		payload.Nodes = nodes
		log.Info("nodes collected", zap.Int("count", len(nodes.Nodes)))
	}

	shards, err := collectShards(ctx, client)
	if err != nil {
		log.Warn("shards collection failed", zap.Error(err))
	} else {
		payload.Shards = shards
		log.Info("shards collected", zap.Int("unassigned", shards.UnassignedCount))
	}

	indices, err := collectIndices(ctx, client)
	if err != nil {
		log.Warn("indices collection failed", zap.Error(err))
	} else {
		payload.Indices = indices
		log.Info("indices collected", zap.Int("count", len(indices.Indices)))
	}

	perf, err := collectPerformance(ctx, client)
	if err != nil {
		log.Warn("performance collection failed", zap.Error(err))
	} else {
		payload.Performance = perf
		log.Info("performance metrics collected")
	}

	snapshots, err := collectSnapshots(ctx, client)
	if err != nil {
		log.Warn("snapshots collection failed", zap.Error(err))
	} else {
		payload.Snapshots = snapshots
		log.Info("snapshots collected", zap.Int("repos", snapshots.RepositoriesCount))
	}

	ism, err := collectIsmPolicies(ctx, client)
	if err != nil {
		log.Warn("ISM policies collection failed", zap.Error(err))
	} else {
		payload.IsmPolicies = ism
		log.Info("ISM policies collected", zap.Int("count", ism.PoliciesCount))
	}

	security, err := collectSecurity(ctx, client)
	if err != nil {
		log.Warn("security collection failed", zap.Error(err))
	} else {
		payload.Security = security
		log.Info("security config collected")
	}

	plugins, err := collectPlugins(ctx, client)
	if err != nil {
		log.Warn("plugins collection failed", zap.Error(err))
	} else {
		payload.Plugins = plugins
		log.Info("plugins collected", zap.Int("count", len(plugins.Plugins)))
	}

	ingest, err := collectIngestPipelines(ctx, client)
	if err != nil {
		log.Warn("ingest pipelines collection failed", zap.Error(err))
	} else {
		payload.IngestPipelines = ingest
		log.Info("ingest pipelines collected", zap.Int("count", ingest.PipelinesCount))
	}

	templates, err := collectTemplates(ctx, client)
	if err != nil {
		log.Warn("templates collection failed", zap.Error(err))
	} else {
		payload.Templates = templates
		log.Info("templates collected", zap.Int("count", templates.TemplatesCount))
	}

	payload.DurationMs = time.Since(start).Milliseconds()
	return payload, nil
}

func get(ctx context.Context, client *opensearch.Client, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s returned %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ─── Cluster Health ───────────────────────────────────────────

type ClusterHealth struct {
	Status            string `json:"status"`
	NumberOfNodes     int    `json:"numberOfNodes"`
	NumberOfDataNodes int    `json:"numberOfDataNodes"`
	ActiveShards      int    `json:"activeShards"`
	UnassignedShards  int    `json:"unassignedShards"`
	PendingTasks      int    `json:"pendingTasks"`
}

func collectClusterHealth(ctx context.Context, client *opensearch.Client) (*ClusterHealth, error) {
	body, err := get(ctx, client, "/_cluster/health")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Status            string `json:"status"`
		NumberOfNodes     int    `json:"number_of_nodes"`
		NumberOfDataNodes int    `json:"number_of_data_nodes"`
		ActiveShards      int    `json:"active_shards"`
		UnassignedShards  int    `json:"unassigned_shards"`
		PendingTasks      int    `json:"number_of_pending_tasks"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return &ClusterHealth{
		Status: raw.Status, NumberOfNodes: raw.NumberOfNodes,
		NumberOfDataNodes: raw.NumberOfDataNodes, ActiveShards: raw.ActiveShards,
		UnassignedShards: raw.UnassignedShards, PendingTasks: raw.PendingTasks,
	}, nil
}

// ─── Nodes ────────────────────────────────────────────────────

type NodeStat struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Roles            []string `json:"roles"`
	HeapUsedPercent  float64  `json:"heapUsedPercent"`
	CPUPercent       float64  `json:"cpuPercent"`
	DiskUsedPercent  float64  `json:"diskUsedPercent"`
	DiskTotalBytes   int64    `json:"diskTotalBytes"`
	DiskAvailBytes   int64    `json:"diskAvailableBytes"`
	UptimeMs         int64    `json:"uptimeMs"`
	OSMemUsedPercent float64  `json:"osMemUsedPercent"`
}

type NodesData struct {
	Nodes []NodeStat `json:"nodes"`
}

func collectNodes(ctx context.Context, client *opensearch.Client) (*NodesData, error) {
	body, err := get(ctx, client, "/_nodes/stats")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Nodes map[string]struct {
			Name  string   `json:"name"`
			Roles []string `json:"roles"`
			JVM   struct {
				Mem    struct{ HeapUsedPercent float64 `json:"heap_used_percent"` } `json:"mem"`
				Uptime int64 `json:"uptime_in_millis"`
			} `json:"jvm"`
			OS struct {
				CPU struct{ Percent float64 `json:"percent"` } `json:"cpu"`
				Mem struct{ UsedPercent float64 `json:"used_percent"` } `json:"mem"`
			} `json:"os"`
			FS struct {
				Total struct {
					TotalInBytes     int64 `json:"total_in_bytes"`
					AvailableInBytes int64 `json:"available_in_bytes"`
				} `json:"total"`
			} `json:"fs"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	var stats []NodeStat
	for id, n := range raw.Nodes {
		diskTotal := n.FS.Total.TotalInBytes
		diskAvail := n.FS.Total.AvailableInBytes
		var pct float64
		if diskTotal > 0 {
			pct = float64(diskTotal-diskAvail) / float64(diskTotal) * 100
		}
		stats = append(stats, NodeStat{
			ID: id, Name: n.Name, Roles: n.Roles,
			HeapUsedPercent: n.JVM.Mem.HeapUsedPercent,
			CPUPercent:      n.OS.CPU.Percent,
			DiskUsedPercent: pct, DiskTotalBytes: diskTotal, DiskAvailBytes: diskAvail,
			UptimeMs: n.JVM.Uptime, OSMemUsedPercent: n.OS.Mem.UsedPercent,
		})
	}
	return &NodesData{Nodes: stats}, nil
}

// ─── Shards ───────────────────────────────────────────────────

type ShardsData struct {
	UnassignedCount   int            `json:"unassignedCount"`
	UnassignedReasons map[string]int `json:"unassignedReasons"`
	ShardCountPerNode map[string]int `json:"shardCountPerNode"`
	AvgShardSizeBytes float64        `json:"avgShardSizeBytes"`
}

func collectShards(ctx context.Context, client *opensearch.Client) (*ShardsData, error) {
	body, err := get(ctx, client, "/_cat/shards?format=json&bytes=b")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		State       string `json:"state"`
		UnassignedReason string `json:"unassigned.reason"`
		Node        string `json:"node"`
		Store       string `json:"store"` // bytes as string
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	data := &ShardsData{
		UnassignedReasons: map[string]int{},
		ShardCountPerNode: map[string]int{},
	}
	var totalSize float64
	var assignedCount int
	for _, s := range raw {
		if s.State == "UNASSIGNED" {
			data.UnassignedCount++
			if s.UnassignedReason != "" {
				data.UnassignedReasons[s.UnassignedReason]++
			}
		} else {
			if s.Node != "" {
				data.ShardCountPerNode[s.Node]++
			}
			var size float64
			fmt.Sscanf(s.Store, "%f", &size)
			totalSize += size
			assignedCount++
		}
	}
	if assignedCount > 0 {
		data.AvgShardSizeBytes = totalSize / float64(assignedCount)
	}
	return data, nil
}

// ─── Indices ──────────────────────────────────────────────────

type IndexStat struct {
	Name             string `json:"name"`
	Health           string `json:"health"`
	Status           string `json:"status"`
	PrimaryShards    int    `json:"primaryShards"`
	Replicas         int    `json:"replicas"`
	DocsCount        int64  `json:"docsCount"`
	StoreSizeBytes   int64  `json:"storeSizeBytes"`
	MappingFieldCount int   `json:"mappingFieldCount"`
	IsReadOnly       bool   `json:"isReadOnly"`
	RefreshInterval  string `json:"refreshInterval"`
}

type IndicesData struct {
	Indices []IndexStat `json:"indices"`
}

func collectIndices(ctx context.Context, client *opensearch.Client) (*IndicesData, error) {
	body, err := get(ctx, client, "/_cat/indices?format=json&bytes=b&h=index,health,status,pri,rep,docs.count,store.size")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Index     string `json:"index"`
		Health    string `json:"health"`
		Status    string `json:"status"`
		Pri       string `json:"pri"`
		Rep       string `json:"rep"`
		DocsCount string `json:"docs.count"`
		StoreSize string `json:"store.size"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	// Fetch read-only settings
	settingsBody, settingsErr := get(ctx, client, "/_all/_settings/index.blocks.read_only_allow_delete,index.blocks.read_only")

	readOnlySet := map[string]bool{}
	if settingsErr == nil {
		var settings map[string]struct {
			Settings struct {
				Index struct {
					Blocks struct {
						ReadOnly            string `json:"read_only"`
						ReadOnlyAllowDelete string `json:"read_only_allow_delete"`
					} `json:"blocks"`
				} `json:"index"`
			} `json:"settings"`
		}
		if json.Unmarshal(settingsBody, &settings) == nil {
			for name, s := range settings {
				if s.Settings.Index.Blocks.ReadOnly == "true" ||
					s.Settings.Index.Blocks.ReadOnlyAllowDelete == "true" {
					readOnlySet[name] = true
				}
			}
		}
	}

	var indices []IndexStat
	for _, r := range raw {
		var pri, rep int
		var docs, size int64
		fmt.Sscanf(r.Pri, "%d", &pri)
		fmt.Sscanf(r.Rep, "%d", &rep)
		fmt.Sscanf(r.DocsCount, "%d", &docs)
		fmt.Sscanf(r.StoreSize, "%d", &size)
		indices = append(indices, IndexStat{
			Name:           r.Index,
			Health:         r.Health,
			Status:         r.Status,
			PrimaryShards:  pri,
			Replicas:       rep,
			DocsCount:      docs,
			StoreSizeBytes: size,
			IsReadOnly:     readOnlySet[r.Index],
		})
	}
	return &IndicesData{Indices: indices}, nil
}

// ─── Performance ──────────────────────────────────────────────

type PerformanceData struct {
	IndexingRatePerSec float64 `json:"indexingRatePerSec"`
	SearchRatePerSec   float64 `json:"searchRatePerSec"`
	SearchLatencyMs    float64 `json:"searchLatencyMs"`
	QueryRejections    int64   `json:"queryRejections"`
	BulkRejections     int64   `json:"bulkRejections"`
	QueryCacheHitRate  float64 `json:"queryCacheHitRate"`
	FieldDataEvictions int64   `json:"fieldDataEvictions"`
	SegmentCountTotal  int64   `json:"segmentCountTotal"`
	MergeTimeMs        int64   `json:"mergeTimeMs"`
}

func collectPerformance(ctx context.Context, client *opensearch.Client) (*PerformanceData, error) {
	body, err := get(ctx, client, "/_nodes/stats/indices,thread_pool")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Nodes map[string]struct {
			Indices struct {
				Indexing struct {
					IndexTotal int64   `json:"index_total"`
					IndexTimeMs int64  `json:"index_time_in_millis"`
				} `json:"indexing"`
				Search struct {
					QueryTotal   int64 `json:"query_total"`
					QueryTimeMs  int64 `json:"query_time_in_millis"`
					FetchTotal   int64 `json:"fetch_total"`
				} `json:"search"`
				QueryCache struct {
					HitCount  int64 `json:"hit_count"`
					MissCount int64 `json:"miss_count"`
					Evictions int64 `json:"evictions"`
				} `json:"query_cache"`
				Fielddata struct {
					Evictions int64 `json:"evictions"`
				} `json:"fielddata"`
				Segments struct {
					Count     int64 `json:"count"`
					MergeTimeMs int64 `json:"merge_time_in_millis"`
				} `json:"segments"`
			} `json:"indices"`
			ThreadPool struct {
				Write struct {
					Rejected int64 `json:"rejected"`
				} `json:"write"`
				Search struct {
					Rejected int64 `json:"rejected"`
				} `json:"search"`
			} `json:"thread_pool"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	data := &PerformanceData{}
	for _, n := range raw.Nodes {
		data.BulkRejections += n.ThreadPool.Write.Rejected
		data.QueryRejections += n.ThreadPool.Search.Rejected
		data.FieldDataEvictions += n.Indices.Fielddata.Evictions
		data.SegmentCountTotal += n.Indices.Segments.Count
		data.MergeTimeMs += n.Indices.Segments.MergeTimeMs
		// Query cache hit rate
		hits := n.Indices.QueryCache.HitCount
		misses := n.Indices.QueryCache.MissCount
		if hits+misses > 0 {
			data.QueryCacheHitRate = float64(hits) / float64(hits+misses)
		}
		// Search latency
		if n.Indices.Search.QueryTotal > 0 {
			data.SearchLatencyMs = float64(n.Indices.Search.QueryTimeMs) / float64(n.Indices.Search.QueryTotal)
		}
		// Rates (totals — operator should compare across runs for rate)
		data.SearchRatePerSec = float64(n.Indices.Search.QueryTotal)
		data.IndexingRatePerSec = float64(n.Indices.Indexing.IndexTotal)
	}
	return data, nil
}

// ─── Snapshots ────────────────────────────────────────────────

type SnapshotsData struct {
	RepositoriesCount         int     `json:"repositoriesCount"`
	LastSuccessfulSnapshotAt  *string `json:"lastSuccessfulSnapshotAt"`
	FailedSnapshotsLast7Days  int     `json:"failedSnapshotsLast7Days"`
}

func collectSnapshots(ctx context.Context, client *opensearch.Client) (*SnapshotsData, error) {
	data := &SnapshotsData{}

	// List repositories
	reposBody, err := get(ctx, client, "/_snapshot")
	if err != nil {
		return data, nil // no snapshots configured is valid
	}
	var repos map[string]interface{}
	if json.Unmarshal(reposBody, &repos) == nil {
		data.RepositoriesCount = len(repos)
	}
	if data.RepositoriesCount == 0 {
		return data, nil
	}

	// Get all snapshots from all repos
	snapsBody, err := get(ctx, client, "/_snapshot/_all/_all?ignore_unavailable=true")
	if err != nil {
		return data, nil
	}
	var snapsRaw struct {
		Snapshots []struct {
			State   string `json:"state"`
			EndTime string `json:"end_time"`
		} `json:"snapshots"`
	}
	if err := json.Unmarshal(snapsBody, &snapsRaw); err != nil {
		return data, nil
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var latestSuccess time.Time
	for _, s := range snapsRaw.Snapshots {
		t, err := time.Parse(time.RFC3339, s.EndTime)
		if err != nil {
			continue
		}
		if s.State == "SUCCESS" && t.After(latestSuccess) {
			latestSuccess = t
		}
		if s.State == "FAILED" && t.After(cutoff) {
			data.FailedSnapshotsLast7Days++
		}
	}
	if !latestSuccess.IsZero() {
		ts := latestSuccess.UTC().Format(time.RFC3339)
		data.LastSuccessfulSnapshotAt = &ts
	}
	return data, nil
}

// ─── ISM Policies ─────────────────────────────────────────────

type IsmPoliciesData struct {
	PoliciesCount      int `json:"policiesCount"`
	IndicesWithoutPolicy int `json:"indicesWithoutPolicy"`
	IndicesWithErrors  int `json:"indicesWithErrors"`
}

func collectIsmPolicies(ctx context.Context, client *opensearch.Client) (*IsmPoliciesData, error) {
	data := &IsmPoliciesData{}

	body, err := get(ctx, client, "/_plugins/_ism/policies")
	if err != nil {
		return data, nil // ISM plugin may not be installed
	}
	var raw struct {
		Policies []interface{} `json:"policies"`
	}
	if json.Unmarshal(body, &raw) == nil {
		data.PoliciesCount = len(raw.Policies)
	}

	// Check managed indices for errors
	explainBody, err := get(ctx, client, "/_plugins/_ism/explain/*")
	if err != nil {
		return data, nil
	}
	var explain struct {
		Indices map[string]struct {
			Index  string `json:"index"`
			Policy string `json:"policy_id"`
			Info   struct {
				Message string `json:"message"`
				Cause   string `json:"cause"`
			} `json:"info"`
		} `json:"indices"`
	}
	if json.Unmarshal(explainBody, &explain) == nil {
		for _, idx := range explain.Indices {
			if idx.Policy == "" {
				data.IndicesWithoutPolicy++
			}
			if idx.Info.Cause != "" {
				data.IndicesWithErrors++
			}
		}
	}
	return data, nil
}

// ─── Security ─────────────────────────────────────────────────

type SecurityData struct {
	TLSHTTPEnabled        bool `json:"tlsHttpEnabled"`
	TLSTransportEnabled   bool `json:"tlsTransportEnabled"`
	AnonymousAccessEnabled bool `json:"anonymousAccessEnabled"`
	AuditLoggingEnabled   bool `json:"auditLoggingEnabled"`
	AuthBackendConfigured bool `json:"authBackendConfigured"`
}

func collectSecurity(ctx context.Context, client *opensearch.Client) (*SecurityData, error) {
	data := &SecurityData{}

	body, err := get(ctx, client, "/_plugins/_security/api/ssl/certs")
	if err == nil {
		var ssl struct {
			HttpCertificates      []interface{} `json:"http_certificates_list"`
			TransportCertificates []interface{} `json:"transport_certificates_list"`
		}
		if json.Unmarshal(body, &ssl) == nil {
			data.TLSHTTPEnabled = len(ssl.HttpCertificates) > 0
			data.TLSTransportEnabled = len(ssl.TransportCertificates) > 0
		}
	}

	// Check audit config
	auditBody, err := get(ctx, client, "/_plugins/_security/api/audit")
	if err == nil {
		var audit struct {
			Config struct {
				Enabled bool `json:"enabled"`
			} `json:"config"`
		}
		if json.Unmarshal(auditBody, &audit) == nil {
			data.AuditLoggingEnabled = audit.Config.Enabled
		}
	}

	// Check auth config
	authBody, err := get(ctx, client, "/_plugins/_security/api/securityconfig")
	if err == nil {
		var authCfg map[string]interface{}
		if json.Unmarshal(authBody, &authCfg) == nil {
			data.AuthBackendConfigured = len(authCfg) > 0
		}
	}

	return data, nil
}

// ─── Plugins ──────────────────────────────────────────────────

type PluginInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type PluginsData struct {
	OSVersion string       `json:"osVersion"`
	Plugins   []PluginInfo `json:"plugins"`
}

func collectPlugins(ctx context.Context, client *opensearch.Client) (*PluginsData, error) {
	body, err := get(ctx, client, "/_cat/plugins?format=json&h=name,component,version")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name      string `json:"name"` // node name
		Component string `json:"component"`
		Version   string `json:"version"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	// Deduplicate by component name
	seen := map[string]bool{}
	data := &PluginsData{}
	for _, p := range raw {
		if !seen[p.Component] {
			seen[p.Component] = true
			data.Plugins = append(data.Plugins, PluginInfo{Name: p.Component, Version: p.Version})
		}
	}

	// Get OS version
	infoBody, err := get(ctx, client, "/")
	if err == nil {
		var info struct {
			Version struct {
				Number string `json:"number"`
			} `json:"version"`
		}
		if json.Unmarshal(infoBody, &info) == nil {
			data.OSVersion = info.Version.Number
		}
	}
	return data, nil
}

// ─── Ingest Pipelines ─────────────────────────────────────────

type IngestData struct {
	PipelinesCount   int `json:"pipelinesCount"`
	OrphanedPipelines int `json:"orphanedPipelines"`
}

func collectIngestPipelines(ctx context.Context, client *opensearch.Client) (*IngestData, error) {
	body, err := get(ctx, client, "/_ingest/pipeline")
	if err != nil {
		return &IngestData{}, nil
	}
	var pipelines map[string]interface{}
	if err := json.Unmarshal(body, &pipelines); err != nil {
		return &IngestData{}, nil
	}
	data := &IngestData{PipelinesCount: len(pipelines)}

	// Find which pipelines are referenced by index settings
	settingsBody, err := get(ctx, client, "/_all/_settings/index.default_pipeline,index.final_pipeline")
	if err != nil {
		return data, nil
	}
	var settings map[string]struct {
		Settings struct {
			Index struct {
				DefaultPipeline string `json:"default_pipeline"`
				FinalPipeline   string `json:"final_pipeline"`
			} `json:"index"`
		} `json:"settings"`
	}
	if json.Unmarshal(settingsBody, &settings) != nil {
		return data, nil
	}
	referenced := map[string]bool{}
	for _, s := range settings {
		if s.Settings.Index.DefaultPipeline != "" {
			referenced[s.Settings.Index.DefaultPipeline] = true
		}
		if s.Settings.Index.FinalPipeline != "" {
			referenced[s.Settings.Index.FinalPipeline] = true
		}
	}
	for name := range pipelines {
		if !referenced[name] {
			data.OrphanedPipelines++
		}
	}
	return data, nil
}

// ─── Index Templates ──────────────────────────────────────────

type TemplatesData struct {
	TemplatesCount       int `json:"templatesCount"`
	OverlappingPriorities int `json:"overlappingPriorities"`
	UnusedTemplates      int `json:"unusedTemplates"`
}

func collectTemplates(ctx context.Context, client *opensearch.Client) (*TemplatesData, error) {
	body, err := get(ctx, client, "/_index_template")
	if err != nil {
		return &TemplatesData{}, nil
	}
	var raw struct {
		IndexTemplates []struct {
			Name         string `json:"name"`
			IndexTemplate struct {
				IndexPatterns []string `json:"index_patterns"`
				Priority      int      `json:"priority"`
			} `json:"index_template"`
		} `json:"index_templates"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return &TemplatesData{}, nil
	}

	data := &TemplatesData{TemplatesCount: len(raw.IndexTemplates)}

	// Detect overlapping patterns at the same priority
	type patternKey struct {
		pattern  string
		priority int
	}
	patternCount := map[patternKey]int{}
	for _, t := range raw.IndexTemplates {
		for _, p := range t.IndexTemplate.IndexPatterns {
			patternCount[patternKey{p, t.IndexTemplate.Priority}]++
		}
	}
	for _, count := range patternCount {
		if count > 1 {
			data.OverlappingPriorities++
		}
	}

	// Check which templates are unused (no matching index)
	indicesBody, err := get(ctx, client, "/_cat/indices?format=json&h=index")
	if err != nil {
		return data, nil
	}
	var indices []struct {
		Index string `json:"index"`
	}
	if json.Unmarshal(indicesBody, &indices) != nil {
		return data, nil
	}
	indexNames := make([]string, 0, len(indices))
	for _, i := range indices {
		indexNames = append(indexNames, i.Index)
	}

	for _, t := range raw.IndexTemplates {
		matched := false
		for _, pattern := range t.IndexTemplate.IndexPatterns {
			for _, idx := range indexNames {
				if matchesPattern(pattern, idx) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			data.UnusedTemplates++
		}
	}
	return data, nil
}

// matchesPattern does simple wildcard matching (only * supported).
func matchesPattern(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	// Simple prefix/suffix wildcard
	if len(pattern) == 0 {
		return name == ""
	}
	if pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(name) >= len(prefix) && name[:len(prefix)] == prefix
	}
	if pattern[0] == '*' {
		suffix := pattern[1:]
		return len(name) >= len(suffix) && name[len(name)-len(suffix):] == suffix
	}
	return pattern == name
}
