// Package thunderid implements idp.ApplicationManager against ThunderID's
// management API: Portal Backend creates each data-consumer application as an
// M2M client in its member's organization unit and grants it the data-consumer
// role.
package thunderid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"golang.org/x/oauth2/clientcredentials"

	"github.com/openndx/openndx-core/internal/pb/idp"
)

// Config configures the ThunderID client.
type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	// Scopes requested for the management API token, e.g. "system".
	Scopes []string
	// Resource is sent as the token request's `resource` parameter: the
	// identifier of ThunderID's System resource server, which owns the
	// `system` scope. ThunderID rejects the token request without it.
	Resource string
	// DataConsumerRoleID is the role every created application is assigned,
	// so it can request the `getdata` scope. Empty skips the assignment.
	DataConsumerRoleID string
}

type Client struct {
	baseURL            string
	dataConsumerRoleID string
	httpClient         *http.Client
}

var _ idp.ApplicationManager = (*Client)(nil)

func NewClient(cfg Config) *Client {
	oauthConfig := &clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.BaseURL + "/oauth2/token",
		Scopes:       cfg.Scopes,
	}
	if cfg.Resource != "" {
		oauthConfig.EndpointParams = url.Values{"resource": {cfg.Resource}}
	}

	return &Client{
		baseURL:            cfg.BaseURL,
		dataConsumerRoleID: cfg.DataConsumerRoleID,
		httpClient:         oauthConfig.Client(context.Background()),
	}
}

type oauthConfig struct {
	ClientID                string   `json:"clientId,omitempty"`
	ClientSecret            string   `json:"clientSecret,omitempty"`
	GrantTypes              []string `json:"grantTypes,omitempty"`
	TokenEndpointAuthMethod string   `json:"tokenEndpointAuthMethod,omitempty"`
	PublicClient            bool     `json:"publicClient"`
}

type inboundAuthConfig struct {
	Type   string      `json:"type"`
	Config oauthConfig `json:"config"`
}

type applicationRequest struct {
	Name              string              `json:"name"`
	Description       string              `json:"description,omitempty"`
	Type              string              `json:"type"`
	OuID              string              `json:"ouId"`
	InboundAuthConfig []inboundAuthConfig `json:"inboundAuthConfig"`
}

type applicationResponse struct {
	ID                string              `json:"id"`
	ClientID          string              `json:"clientId"`
	InboundAuthConfig []inboundAuthConfig `json:"inboundAuthConfig"`
}

type assignment struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type assignmentsRequest struct {
	Assignments []assignment `json:"assignments"`
}

// CreateApplication creates a confidential client_credentials client in
// app.OuID and assigns it the data-consumer role. If the assignment fails, the
// application is deleted again. ThunderID generates the client ID and secret
// and only returns the secret here.
func (c *Client) CreateApplication(ctx context.Context, app *idp.Application) (*idp.CreatedApplication, error) {
	if app.OuID == "" {
		return nil, fmt.Errorf("thunderid: application organization unit is required")
	}

	body := applicationRequest{
		Name:        app.Name,
		Description: app.Description,
		Type:        "m2m",
		OuID:        app.OuID,
		InboundAuthConfig: []inboundAuthConfig{{
			Type: "oauth2",
			Config: oauthConfig{
				GrantTypes:              []string{"client_credentials"},
				TokenEndpointAuthMethod: "client_secret_basic",
				PublicClient:            false,
			},
		}},
	}

	var created applicationResponse
	if err := c.do(ctx, http.MethodPost, "/applications", body, http.StatusCreated, &created); err != nil {
		return nil, err
	}

	result := &idp.CreatedApplication{Id: created.ID, ClientId: created.ClientID}
	for _, cfg := range created.InboundAuthConfig {
		if cfg.Type == "oauth2" {
			result.ClientSecret = cfg.Config.ClientSecret
		}
	}

	if c.dataConsumerRoleID != "" {
		assign := assignmentsRequest{Assignments: []assignment{{Type: "app", ID: created.ID}}}
		if err := c.do(ctx, http.MethodPost, "/roles/"+url.PathEscape(c.dataConsumerRoleID)+"/assignments/add", assign, http.StatusNoContent, nil); err != nil {
			if deleteErr := c.DeleteApplication(ctx, created.ID); deleteErr != nil {
				return nil, fmt.Errorf("failed to assign data-consumer role: %w, and failed to delete application: %w", err, deleteErr)
			}
			return nil, fmt.Errorf("failed to assign data-consumer role: %w", err)
		}
	}

	return result, nil
}

func (c *Client) DeleteApplication(ctx context.Context, applicationId string) error {
	return c.do(ctx, http.MethodDelete, "/applications/"+url.PathEscape(applicationId), nil, http.StatusNoContent, nil)
}

// apiError is ThunderID's error body.
type apiError struct {
	Code    string `json:"code"`
	Message struct {
		DefaultValue string `json:"defaultValue"`
	} `json:"message"`
	Description struct {
		DefaultValue string `json:"defaultValue"`
	} `json:"description"`
}

// do sends a JSON request and decodes the response into out (if non-nil). Any
// status other than want is returned as an error carrying ThunderID's message.
func (c *Client) do(ctx context.Context, method, path string, in any, want int, out any) error {
	var reqBody io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != want {
		var apiErr apiError
		if json.NewDecoder(res.Body).Decode(&apiErr) == nil && apiErr.Message.DefaultValue != "" {
			return fmt.Errorf("thunderid %s %s: %s (%s): %s", method, path, apiErr.Message.DefaultValue, apiErr.Code, apiErr.Description.DefaultValue)
		}
		return fmt.Errorf("thunderid %s %s: request failed with status %s", method, path, res.Status)
	}

	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}
