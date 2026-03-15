package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/opensearch-doctor/agent/internal/collector"
	"github.com/opensearch-doctor/agent/internal/config"
	"github.com/opensearch-doctor/agent/internal/sender"
)

const AgentVersion = "0.1.0"

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	runOnce := flag.Bool("once", false, "run diagnostics once and exit")
	testMode := flag.Bool("test", false, "run diagnostics, print results, do NOT send to SaaS")
	flag.Parse()

	log, _ := zap.NewProduction()
	defer log.Sync() //nolint:errcheck

	log.Info("OpenSearch Doctor Agent starting", zap.String("version", AgentVersion))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	log.Info("config loaded",
		zap.String("cluster", cfg.Cluster.Name),
		zap.String("endpoint", cfg.Cluster.Endpoint),
	)

	osClient, err := collector.NewOSClient(&cfg.Cluster)
	if err != nil {
		log.Fatal("failed to create OpenSearch client", zap.Error(err))
	}

	send := sender.New(cfg.SaaS.APIURL, cfg.SaaS.APIKey, log)

	// Register cluster and get cluster ID (unless test mode)
	var clusterID string
	if !*testMode {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		clusterUUID := collector.GetClusterUUID(ctx, osClient)
		clusterID, err = send.Register(ctx, cfg.Cluster.Name, cfg.Cluster.Endpoint,
			cfg.Cluster.Environment, "", AgentVersion, clusterUUID)
		cancel()
		if err != nil {
			log.Fatal("cluster registration failed", zap.Error(err))
		}
		log.Info("cluster registered",
			zap.String("clusterId", clusterID),
			zap.String("clusterUuid", clusterUUID),
		)
	}

	runDiagnostics := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		log.Info("starting diagnostic collection")
		payload, err := collector.Collect(ctx, osClient, log)
		if err != nil {
			log.Error("collection error", zap.Error(err))
			return
		}

		if *testMode {
			fmt.Printf("\n=== Diagnostic Results (test mode) ===\n")
			if payload.ClusterHealth != nil {
				fmt.Printf("Cluster status: %s | nodes: %d | unassigned shards: %d\n",
					payload.ClusterHealth.Status,
					payload.ClusterHealth.NumberOfNodes,
					payload.ClusterHealth.UnassignedShards,
				)
			}
			if payload.Nodes != nil {
				for _, n := range payload.Nodes.Nodes {
					fmt.Printf("  Node %-20s heap: %.0f%%  cpu: %.0f%%  disk: %.0f%%\n",
						n.Name, n.HeapUsedPercent, n.CPUPercent, n.DiskUsedPercent)
				}
			}
			fmt.Printf("Duration: %dms\n\n", payload.DurationMs)
			return
		}

		sessionID, healthScore, err := send.SendDiagnostics(ctx, clusterID, AgentVersion,
			"", payload.DurationMs, payload)
		if err != nil {
			log.Error("failed to send diagnostics", zap.Error(err))
			return
		}

		log.Info("diagnostic run complete",
			zap.String("sessionId", sessionID),
			zap.Int("healthScore", healthScore),
		)
	}

	// Run once immediately
	runDiagnostics()

	if *runOnce || *testMode {
		log.Info("--once flag set, exiting")
		os.Exit(0)
	}

	// Start heartbeat + command polling goroutine
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.Agent.HeartbeatSeconds) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := send.Heartbeat(ctx, clusterID, AgentVersion); err != nil {
				log.Warn("heartbeat failed", zap.Error(err))
			}
			cancel()

			// Poll and execute pending remediation commands
			cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 30*time.Second)
			commands, err := send.PollCommands(cmdCtx)
			cmdCancel()
			if err != nil {
				log.Warn("failed to poll commands", zap.Error(err))
				continue
			}
			for _, cmd := range commands {
				log.Info("executing remediation command",
					zap.String("id", cmd.ID),
					zap.String("label", cmd.Label),
					zap.String("method", cmd.Method),
					zap.String("path", cmd.Path),
				)
				result, execErr := executeRemediationCommand(&cfg.Cluster, cmd)
				ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
				errMsg := ""
				if execErr != nil {
					errMsg = execErr.Error()
				}
				if err := send.ReportCommandResult(ctx2, cmd.ID, execErr == nil, result, errMsg); err != nil {
					log.Warn("failed to report command result", zap.Error(err))
				}
				cancel2()
				if execErr != nil {
					log.Warn("remediation command failed", zap.String("id", cmd.ID), zap.Error(execErr))
				} else {
					log.Info("remediation command completed", zap.String("id", cmd.ID), zap.String("result", result))
				}
			}
		}
	}()

	// Schedule recurring diagnostic runs
	interval := time.Duration(cfg.Agent.IntervalMinutes) * time.Minute
	log.Info("scheduling diagnostics", zap.Duration("interval", interval))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Wait for interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			runDiagnostics()
		case <-quit:
			log.Info("agent shutting down")
			return
		}
	}
}

// executeRemediationCommand runs a single remediation API call against the OpenSearch cluster.
func executeRemediationCommand(cluster *config.ClusterConfig, cmd sender.RemediationCommand) (string, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: cluster.TLSSkipVerify} //nolint:gosec
	httpClient := &http.Client{
		Timeout:   20 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}

	var bodyReader io.Reader
	if cmd.Body != "" {
		bodyReader = bytes.NewBufferString(cmd.Body)
	}

	req, err := http.NewRequest(cmd.Method, cluster.Endpoint+cmd.Path, bodyReader)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	if cmd.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cluster.APIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+cluster.APIKey)
	} else {
		req.SetBasicAuth(cluster.Username, cluster.Password)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing %s %s: %w", cmd.Method, cmd.Path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s %s returned %d: %s", cmd.Method, cmd.Path, resp.StatusCode, string(respBody))
	}
	return string(respBody), nil
}
