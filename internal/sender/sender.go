package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Sender struct {
	apiURL string
	apiKey string
	client *http.Client
	log    *zap.Logger
}

func New(apiURL, apiKey string, log *zap.Logger) *Sender {
	return &Sender{
		apiURL: apiURL,
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
		log:    log,
	}
}

// Register calls /api/agent/register and returns the cluster ID.
func (s *Sender) Register(ctx context.Context, clusterName, endpoint, environment, osVersion, agentVersion, clusterUUID string) (string, error) {
	payload := map[string]string{
		"clusterName":  clusterName,
		"endpoint":     endpoint,
		"environment":  environment,
		"osVersion":    osVersion,
		"agentVersion": agentVersion,
		"clusterUuid":  clusterUUID,
	}

	var result struct {
		ClusterID string `json:"clusterId"`
	}
	if err := s.post(ctx, "/api/agent/register", payload, &result); err != nil {
		return "", err
	}

	s.log.Info("cluster registered", zap.String("clusterId", result.ClusterID))
	return result.ClusterID, nil
}

// Heartbeat updates the agent's last-seen timestamp.
func (s *Sender) Heartbeat(ctx context.Context, clusterID, agentVersion string) error {
	return s.post(ctx, "/api/agent/heartbeat", map[string]string{
		"clusterId":    clusterID,
		"agentVersion": agentVersion,
	}, nil)
}

// SendDiagnostics posts the collected diagnostic payload.
func (s *Sender) SendDiagnostics(ctx context.Context, clusterID, agentVersion, osVersion string, durationMs int64, data any) (string, int, error) {
	payload := map[string]any{
		"clusterId":    clusterID,
		"agentVersion": agentVersion,
		"osVersion":    osVersion,
		"durationMs":   durationMs,
		"data":         data,
	}

	var result struct {
		SessionID   string `json:"sessionId"`
		HealthScore int    `json:"healthScore"`
	}
	if err := s.post(ctx, "/api/agent/diagnostics", payload, &result); err != nil {
		return "", 0, err
	}

	s.log.Info("diagnostics sent",
		zap.String("sessionId", result.SessionID),
		zap.Int("healthScore", result.HealthScore),
	)
	return result.SessionID, result.HealthScore, nil
}

// RemediationCommand represents a pending fix sent from the platform.
type RemediationCommand struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body"`
}

// PollCommands fetches pending remediation commands from the platform.
func (s *Sender) PollCommands(ctx context.Context) ([]RemediationCommand, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiURL+"/api/agent/commands", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /api/agent/commands: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET /api/agent/commands returned %d", resp.StatusCode)
	}
	var result struct {
		Commands []RemediationCommand `json:"commands"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	return result.Commands, nil
}

// ReportCommandResult posts the execution result back to the platform.
func (s *Sender) ReportCommandResult(ctx context.Context, commandID string, success bool, result, errMsg string) error {
	payload := map[string]any{
		"success": success,
		"result":  result,
		"error":   errMsg,
	}
	return s.post(ctx, "/api/agent/commands/"+commandID+"/result", payload, nil)
}

func (s *Sender) post(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("POST %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("parsing response from %s: %w", path, err)
		}
	}

	return nil
}
