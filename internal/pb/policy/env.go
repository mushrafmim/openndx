package policy

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// NewClientFromEnv creates a PDP client for the URL in PDP_SERVICE_URL, which
// must be set and use the http:// or https:// scheme.
func NewClientFromEnv() (*Client, error) {
	pdpServiceURL := os.Getenv("PDP_SERVICE_URL")
	if pdpServiceURL == "" {
		return nil, fmt.Errorf("PDP_SERVICE_URL environment variable not set")
	}
	if !strings.HasPrefix(pdpServiceURL, "http://") && !strings.HasPrefix(pdpServiceURL, "https://") {
		return nil, fmt.Errorf("PDP_SERVICE_URL must start with http:// or https://")
	}

	slog.Info("PDP Service URL", "url", pdpServiceURL)
	return NewClient(pdpServiceURL), nil
}
