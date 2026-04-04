// Package fcprovider implements SandboxProvider using Aliyun FunctionCompute Session API.
package fcprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	fc20230330 "github.com/alibabacloud-go/fc-20230330/v4/client"
	dara "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"

	"github.com/e2b-dev/infra/packages/shared/pkg/provider"
)

// Config contains the configuration for FC provider.
type Config struct {
	// AccessKeyID is the Aliyun access key ID
	AccessKeyID string

	// AccessKeySecret is the Aliyun access key secret
	AccessKeySecret string

	// AccountID is the Aliyun account ID
	AccountID string

	// Region is the FC region (e.g., "cn-shanghai")
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
		Region:             getEnvOrDefault("FC_REGION", "cn-shanghai"),
		DefaultTTL:         3600, // 1 hour
		DefaultIdleTimeout: 300,  // 5 minutes (FC max is 300)
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// sessionMeta stores session metadata for tracking
type sessionMeta struct {
	functionName string
	createdAt    time.Time
}

// FCProvider implements SandboxProvider using Aliyun FC Session API.
type FCProvider struct {
	client   *fc20230330.Client
	config   *Config
	endpoint string
	sessions sync.Map // map[string]*sessionMeta - sandboxID -> session metadata
}

// New creates a new FC provider.
func New(config *Config) (*FCProvider, error) {
	if config == nil {
		config = ConfigFromEnv()
	}

	// Create FC client
	endpoint := fmt.Sprintf("%s.%s.fc.aliyuncs.com", config.AccountID, config.Region)

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

	// Build the request body
	request := &fc20230330.CreateSessionRequest{
		Body: &fc20230330.CreateSessionInput{
			SessionId:                   tea.String(sessionID),
			SessionTTLInSeconds:         tea.Int64(timeout),
			SessionIdleTimeoutInSeconds: tea.Int64(p.config.DefaultIdleTimeout),
		},
	}

	// Create the session
	runtime := &dara.RuntimeOptions{}
	headers := make(map[string]*string)
	response, err := p.client.CreateSessionWithOptions(tea.String(functionName), request, headers, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Track session metadata for later use (Get/Delete need functionName)
	p.sessions.Store(sessionID, &sessionMeta{
		functionName: functionName,
		createdAt:    time.Now(),
	})

	now := time.Now()
	info := &provider.SandboxInfo{
		SandboxID:  sessionID,
		TemplateID: config.TemplateID,
		State:      provider.SandboxStateRunning,
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Duration(timeout) * time.Second),
		Metadata:   config.Metadata,
	}

	_ = response // Response contains session details

	return info, nil
}

// Get returns information about a sandbox.
func (p *FCProvider) Get(ctx context.Context, sandboxID string) (*provider.SandboxInfo, error) {
	// Get the function name from tracked sessions
	meta, ok := p.sessions.Load(sandboxID)
	if !ok {
		return nil, provider.ErrSandboxNotFound
	}
	sessionMeta := meta.(*sessionMeta)
	functionName := sessionMeta.functionName

	request := &fc20230330.GetSessionRequest{}
	runtime := &dara.RuntimeOptions{}
	headers := make(map[string]*string)

	// GetSessionWithOptions requires: functionName, sessionId, request, headers, runtime
	response, err := p.client.GetSessionWithOptions(tea.String(functionName), tea.String(sandboxID), request, headers, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Map FC session status to sandbox state
	state := provider.SandboxStateRunning
	if response.Body != nil && response.Body.SessionStatus != nil {
		switch *response.Body.SessionStatus {
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
	if response.Body != nil && response.Body.CreatedTime != nil {
		createdAt, _ = time.Parse(time.RFC3339, *response.Body.CreatedTime)
	}

	return &provider.SandboxInfo{
		SandboxID:  sandboxID,
		TemplateID: functionName,
		State:      state,
		CreatedAt:  createdAt,
	}, nil
}

// Delete deletes a sandbox.
func (p *FCProvider) Delete(ctx context.Context, sandboxID string) error {
	// Get the function name from tracked sessions
	meta, ok := p.sessions.Load(sandboxID)
	if !ok {
		return provider.ErrSandboxNotFound
	}
	sessionMeta := meta.(*sessionMeta)
	functionName := sessionMeta.functionName

	request := &fc20230330.DeleteSessionRequest{}
	runtime := &dara.RuntimeOptions{}
	headers := make(map[string]*string)

	// DeleteSessionWithOptions requires: functionName, sessionId, request, headers, runtime
	_, err := p.client.DeleteSessionWithOptions(tea.String(functionName), tea.String(sandboxID), request, headers, runtime)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Remove from tracking
	p.sessions.Delete(sandboxID)

	return nil
}

// List returns a paginated list of sandboxes.
// Note: FC Session API requires functionName to list sessions.
// This implementation lists sessions for all tracked functions.
func (p *FCProvider) List(ctx context.Context, filter *provider.ListFilter) (*provider.ListResult, error) {
	result := &provider.ListResult{
		Sandboxes: make([]*provider.SandboxInfo, 0),
	}

	// Collect unique function names from tracked sessions
	functionNames := make(map[string]bool)
	p.sessions.Range(func(key, value interface{}) bool {
		meta := value.(*sessionMeta)
		functionNames[meta.functionName] = true
		return true
	})

	runtime := &dara.RuntimeOptions{}
	headers := make(map[string]*string)

	// List sessions for each function
	for functionName := range functionNames {
		request := &fc20230330.ListSessionsRequest{}

		if filter != nil && filter.Limit > 0 {
			request.Limit = tea.Int32(filter.Limit)
		}

		if filter != nil && filter.NextToken != "" {
			request.NextToken = tea.String(filter.NextToken)
		}

		response, err := p.client.ListSessionsWithOptions(tea.String(functionName), request, headers, runtime)
		if err != nil {
			continue // Skip functions that fail
		}

		if response.Body != nil && response.Body.Sessions != nil {
			for _, session := range response.Body.Sessions {
				state := provider.SandboxStateRunning
				if session.SessionStatus != nil {
					switch *session.SessionStatus {
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
				if session.CreatedTime != nil {
					createdAt, _ = time.Parse(time.RFC3339, *session.CreatedTime)
				}

				result.Sandboxes = append(result.Sandboxes, &provider.SandboxInfo{
					SandboxID:  getValue(session.SessionId),
					TemplateID: functionName,
					State:      state,
					CreatedAt:  createdAt,
				})
			}
		}

		if response.Body != nil && response.Body.NextToken != nil {
			result.NextToken = *response.Body.NextToken
		}
	}

	return result, nil
}

// Connect returns connection info for a sandbox.
func (p *FCProvider) Connect(ctx context.Context, sandboxID string) (*provider.ConnectionInfo, error) {
	// Get the function name from tracked sessions
	meta, ok := p.sessions.Load(sandboxID)
	if !ok {
		return nil, provider.ErrSandboxNotFound
	}
	sessionMeta := meta.(*sessionMeta)
	functionName := sessionMeta.functionName

	// Build the FC function invocation URL
	// Format: https://{account-id}.{region}.fc.aliyuncs.com/2016-08-15/proxy/{function-name}/
	// Requests to this URL with x-session-id header will be routed to the session's instance
	endpoint := fmt.Sprintf("https://%s.%s.fc.aliyuncs.com/2016-08-15/proxy/%s/", p.config.AccountID, p.config.Region, functionName)

	// The access token for envd authentication
	// In FC Session, this is typically provided via the session's environment or metadata
	accessToken := fmt.Sprintf("fc-session-%s", sandboxID)

	return &provider.ConnectionInfo{
		Endpoint:    endpoint,
		AccessToken: accessToken,
		SessionID:   sandboxID,
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