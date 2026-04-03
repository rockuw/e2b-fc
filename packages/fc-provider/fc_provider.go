// Package fcprovider implements SandboxProvider using Aliyun FunctionCompute Session API.
package fcprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	fc20230330 "github.com/alibabacloud-go/fc-20230330-3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"

	"github.com/e2b-dev/infra/packages/api/internal/provider"
)

// Config contains the configuration for FC provider.
type Config struct {
	// AccessKeyID is the Aliyun access key ID
	AccessKeyID string

	// AccessKeySecret is the Aliyun access key secret
	AccessKeySecret string

	// AccountID is the Aliyun account ID
	AccountID string

	// Region is the FC region (e.g., "cn-hangzhou")
	Region string

	// DefaultTTL is the default session TTL in seconds
	DefaultTTL int64

	// DefaultIdleTimeout is the default session idle timeout in seconds
	DefaultIdleTimeout int64
}

// ConfigFromEnv creates a Config from environment variables.
func ConfigFromEnv() *Config {
	return &Config{
		AccessKeyID:        os.Getenv("FC_ACCESS_KEY_ID"),
		AccessKeySecret:    os.Getenv("FC_ACCESS_KEY_SECRET"),
		AccountID:          os.Getenv("FC_ACCOUNT_ID"),
		Region:             getEnvOrDefault("FC_REGION", "cn-hangzhou"),
		DefaultTTL:         3600,  // 1 hour
		DefaultIdleTimeout: 1800,  // 30 minutes
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// FCProvider implements SandboxProvider using Aliyun FC Session API.
type FCProvider struct {
	client    *fc20230330.Client
	config    *Config
	endpoint  string
}

// New creates a new FC provider.
func New(config *Config) (*FCProvider, error) {
	if config == nil {
		config = ConfigFromEnv()
	}

	// Create FC client
	endpoint := fmt.Sprintf("https://%s.%s.fc.aliyuncs.com", config.AccountID, config.Region)

	clientConfig := &openapi.Config{
		AccessKeyId:     &config.AccessKeyID,
		AccessKeySecret: &config.AccessKeySecret,
		Endpoint:        &endpoint,
	}

	client, err := fc20230330.NewClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create FC client: %w", err)
	}

	return &FCProvider{
		client:   client,
		config:   config,
		endpoint: endpoint,
	}, nil
}

// Create creates a new sandbox (FC session).
func (p *FCProvider) Create(ctx context.Context, config *provider.SandboxConfig) (*provider.SandboxInfo, error) {
	// Generate a unique session ID (this becomes the sandbox ID)
	sessionID := generateSessionID()

	// Calculate timeout
	timeout := p.config.DefaultTTL
	if config.Timeout > 0 {
		timeout = int64(config.Timeout.Seconds())
	}

	// Create session request
	// The function name is the template ID
	functionName := config.TemplateID

	request := &fc20230330.CreateSessionRequest{
		FunctionName: &functionName,
		SessionId:    &sessionID,
		SessionTTL:   &timeout,
		// Use isolation mode for complete sandbox isolation
		SessionMode:  tea.String("isolation"),
		// Use HeaderField affinity with x-session-id
		SessionAffinity: tea.String("HeaderField"),
	}

	// Create the session
	runtime := &util.RuntimeOptions{}
	response, err := p.client.CreateSessionWithOptions(request, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	now := time.Now()
	info := &provider.SandboxInfo{
		SandboxID:  sessionID,
		TemplateID: config.TemplateID,
		State:      provider.SandboxStateRunning,
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Duration(timeout) * time.Second),
		Metadata:   config.Metadata,
	}

	// Store session info for tracking (optional, we can also query FC API)
	_ = response // Response contains session details

	return info, nil
}

// Get returns information about a sandbox.
func (p *FCProvider) Get(ctx context.Context, sandboxID string) (*provider.SandboxInfo, error) {
	request := &fc20230330.GetSessionRequest{}
	runtime := &util.RuntimeOptions{}

	response, err := p.client.GetSessionWithOptions(tea.String(sandboxID), request, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Map FC session state to sandbox state
	state := provider.SandboxStateRunning
	if response.Body != nil && response.Body.State != nil {
		switch *response.Body.State {
		case "Running":
			state = provider.SandboxStateRunning
		case "Idle":
			state = provider.SandboxStateIdle
		case "Expired":
			state = provider.SandboxStateExpired
		case "Deleted":
			state = provider.SandboxStateDeleted
		}
	}

	var createdAt, expiresAt time.Time
	if response.Body != nil {
		if response.Body.StartTime != nil {
			createdAt = time.Unix(*response.Body.StartTime/1000, 0)
		}
		if response.Body.EndTime != nil {
			expiresAt = time.Unix(*response.Body.EndTime/1000, 0)
		}
	}

	return &provider.SandboxInfo{
		SandboxID:  sandboxID,
		TemplateID: getValue(response.Body.FunctionName),
		State:      state,
		CreatedAt:  createdAt,
		ExpiresAt:  expiresAt,
	}, nil
}

// Delete deletes a sandbox.
func (p *FCProvider) Delete(ctx context.Context, sandboxID string) error {
	request := &fc20230330.DeleteSessionRequest{}
	runtime := &util.RuntimeOptions{}

	_, err := p.client.DeleteSessionWithOptions(tea.String(sandboxID), request, runtime)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// List returns a paginated list of sandboxes.
func (p *FCProvider) List(ctx context.Context, filter *provider.ListFilter) (*provider.ListResult, error) {
	request := &fc20230330.ListSessionsRequest{}

	if filter != nil && filter.Limit > 0 {
		request.Limit = tea.Int32(filter.Limit)
	}

	if filter != nil && filter.NextToken != "" {
		request.NextToken = tea.String(filter.NextToken)
	}

	runtime := &util.RuntimeOptions{}
	response, err := p.client.ListSessionsWithOptions(request, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	result := &provider.ListResult{
		Sandboxes: make([]*provider.SandboxInfo, 0),
	}

	if response.Body != nil && response.Body.Sessions != nil {
		for _, session := range response.Body.Sessions {
			state := provider.SandboxStateRunning
			if session.State != nil {
				switch *session.State {
				case "Running":
					state = provider.SandboxStateRunning
				case "Idle":
					state = provider.SandboxStateIdle
				case "Expired":
					state = provider.SandboxStateExpired
				case "Deleted":
					state = provider.SandboxStateDeleted
				}
			}

			var createdAt time.Time
			if session.StartTime != nil {
				createdAt = time.Unix(*session.StartTime/1000, 0)
			}

			result.Sandboxes = append(result.Sandboxes, &provider.SandboxInfo{
				SandboxID:  getValue(session.SessionId),
				TemplateID: getValue(session.FunctionName),
				State:      state,
				CreatedAt:  createdAt,
			})
		}
	}

	if response.Body != nil && response.Body.NextToken != nil {
		result.NextToken = *response.Body.NextToken
	}

	return result, nil
}

// Connect returns connection info for a sandbox.
func (p *FCProvider) Connect(ctx context.Context, sandboxID string) (*provider.ConnectionInfo, error) {
	// First verify the sandbox exists
	_, err := p.Get(ctx, sandboxID)
	if err != nil {
		return nil, err
	}

	// Build the session endpoint URL
	// Format: https://{account-id}.{region}.fc.aliyuncs.com/{function-name}?session-id={session-id}
	endpoint := fmt.Sprintf("%s/%s", p.endpoint, sandboxID)

	return &provider.ConnectionInfo{
		Endpoint:     endpoint,
		AccessToken:  "", // FC will provide this via session
		SessionID:    sandboxID,
	}, nil
}

// Helper functions

func generateSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// tea.String is a helper from Aliyun SDK
func tea.String(s string) *string {
	return &s
}