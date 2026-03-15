package collector

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"

	opensearch "github.com/opensearch-project/opensearch-go/v2"

	"github.com/opensearch-doctor/agent/internal/config"
)

// NewOSClient creates an authenticated OpenSearch client from config.
func NewOSClient(cfg *config.ClusterConfig) (*opensearch.Client, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec
	}

	if cfg.CACertPath != "" {
		caCert, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert %s: %w", cfg.CACertPath, err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		tlsCfg.RootCAs = pool
		tlsCfg.InsecureSkipVerify = false
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}

	osCfg := opensearch.Config{
		Addresses: []string{cfg.Endpoint},
		Transport: transport,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}

	return opensearch.NewClient(osCfg)
}
