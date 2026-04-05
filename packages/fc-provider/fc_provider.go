// Package fcprovider implements SandboxProvider using Aliyun FunctionCompute Session API.
package fcprovider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
		DefaultIdleTimeout: 60,   // FC Session requires exactly 60 seconds for sessionIdleTimeoutInSeconds
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
	ttl          int64 // TTL in seconds
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

	// Calculate timeout - FC requires minimum 60 seconds
	timeout := p.config.DefaultTTL
	if config.Timeout > 0 {
		timeout = int64(config.Timeout.Seconds())
	}
	// Enforce FC minimum TTL of 60 seconds
	if timeout < 60 {
		timeout = 60
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
		ttl:          timeout,
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

	// Check if the session has expired based on local TTL tracking
	// FC doesn't automatically expire sessions, so we track TTL locally
	expiresAt := sessionMeta.createdAt.Add(time.Duration(sessionMeta.ttl) * time.Second)
	if time.Now().After(expiresAt) {
		// Session has expired locally, remove from tracking
		p.sessions.Delete(sandboxID)

		return nil, provider.ErrSandboxNotFound
	}

	request := &fc20230330.GetSessionRequest{}
	runtime := &dara.RuntimeOptions{}
	headers := make(map[string]*string)

	// GetSessionWithOptions requires: functionName, sessionId, request, headers, runtime
	response, err := p.client.GetSessionWithOptions(tea.String(functionName), tea.String(sandboxID), request, headers, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Map FC session status to sandbox state
	// FC uses "Active" and "Expired" as status values
	state := provider.SandboxStateRunning
	if response.Body != nil && response.Body.SessionStatus != nil {
		switch *response.Body.SessionStatus {
		case "Active":
			state = provider.SandboxStateRunning
		case "Expired":
			state = provider.SandboxStateExpired
		default:
			// Treat unknown status as running
			state = provider.SandboxStateRunning
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
// This implementation lists sessions for all tracked functions,
// and also includes locally tracked sessions that may not yet appear in FC API.
// Expired and deleted sessions are filtered out.
func (p *FCProvider) List(ctx context.Context, filter *provider.ListFilter) (*provider.ListResult, error) {
	result := &provider.ListResult{
		Sandboxes: make([]*provider.SandboxInfo, 0),
	}

	// Collect unique function names from tracked sessions
	functionNames := make(map[string]bool)
	// Also track sessions we've created locally (may not appear in FC API immediately)
	localSessions := make(map[string]*sessionMeta)
	p.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(string)
		meta := value.(*sessionMeta)
		functionNames[meta.functionName] = true
		localSessions[sessionID] = meta

		return true
	})

	runtime := &dara.RuntimeOptions{}
	headers := make(map[string]*string)

	// Track which session IDs we've already added from FC API
	addedSessionIDs := make(map[string]bool)

	// Get current time for TTL checks
	now := time.Now()

	// List sessions for each function from FC API
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
				sessionID := getValue(session.SessionId)
				addedSessionIDs[sessionID] = true

				// Map FC session status to sandbox state
				// FC uses "Active" and "Expired" as status values
				state := provider.SandboxStateRunning
				if session.SessionStatus != nil {
					switch *session.SessionStatus {
					case "Active":
						state = provider.SandboxStateRunning
					case "Expired":
						state = provider.SandboxStateExpired
					default:
						state = provider.SandboxStateRunning
					}
				}

				// Skip expired sessions from FC
				if state == provider.SandboxStateExpired {
					// Remove from local tracking if present
					p.sessions.Delete(sessionID)

					continue
				}

				// Check local TTL tracking for this session
				if localMeta, ok := localSessions[sessionID]; ok {
					expiresAt := localMeta.createdAt.Add(time.Duration(localMeta.ttl) * time.Second)
					if now.After(expiresAt) {
						// Session has expired locally, remove from tracking
						p.sessions.Delete(sessionID)

						continue
					}
				}

				var createdAt time.Time
				if session.CreatedTime != nil {
					createdAt, _ = time.Parse(time.RFC3339, *session.CreatedTime)
				}

				result.Sandboxes = append(result.Sandboxes, &provider.SandboxInfo{
					SandboxID:  sessionID,
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

	// Add locally tracked sessions that weren't returned by FC API (e.g., newly created)
	// Also check if they should have expired based on TTL
	for sessionID, meta := range localSessions {
		if !addedSessionIDs[sessionID] {
			// Check if this session should still be active based on TTL
			expiresAt := meta.createdAt.Add(time.Duration(meta.ttl) * time.Second)
			if now.After(expiresAt) {
				// Session has expired, remove from tracking
				p.sessions.Delete(sessionID)

				continue
			}

			result.Sandboxes = append(result.Sandboxes, &provider.SandboxInfo{
				SandboxID:  sessionID,
				TemplateID: meta.functionName,
				State:      provider.SandboxStateRunning,
				CreatedAt:  meta.createdAt,
			})
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

// RunCommand executes a command in the sandbox via envd's Process service.
// For sync execution, it waits for the process to complete and returns stdout/stderr.
func (p *FCProvider) RunCommand(ctx context.Context, sandboxID string, config *provider.CommandConfig) (*provider.CommandResult, error) {
	// Get connection info
	connInfo, err := p.Connect(ctx, sandboxID)
	if err != nil {
		return nil, err
	}

	// Build the Process.Start URL
	// Envd Connect RPC endpoint format: /connect.v1.ProcessService/Start
	startURL := strings.TrimSuffix(connInfo.Endpoint, "/") + "/connect.v1.ProcessService/Start"

	// Build the request body matching Connect RPC format
	envs := make(map[string]string)
	for k, v := range config.EnvVars {
		envs[k] = v
	}

	cwd := config.Cwd
	if cwd == "" {
		cwd = "/workspace"
	}

	// Build process config - using the proper Connect RPC format
	processConfig := map[string]interface{}{
		"cmd": config.Command,
		"args": func() []string {
			if config.Args != nil {
				return config.Args
			}
			return []string{}
		}(),
		"envs": envs,
		"cwd":  cwd,
	}

	startRequest := map[string]interface{}{
		"process": processConfig,
	}

	requestBody, err := json.Marshal(startRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create context with timeout if specified
	reqCtx := ctx
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(reqCtx, "POST", startURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers for FC session affinity and Connect RPC
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-Id", connInfo.SessionID)
	req.Header.Set("Connect-Accept-Content-Type", "application/json")
	req.Header.Set("Connect-Protocol", "json")

	// Execute request - timeout is handled by context
	client := &http.Client{
		Timeout: 0, // Let context handle timeout
	}

	resp, err := client.Do(req)
	if err != nil {
		// Check if this was a timeout
		if reqCtx.Err() == context.DeadlineExceeded {
			return &provider.CommandResult{
				ExitCode: 124, // Standard timeout exit code
				Stdout:   "",
				Stderr:   fmt.Sprintf("command timed out after %v", config.Timeout),
			}, nil
		}
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse Connect RPC response (newline-delimited JSON)
	var stdout, stderr bytes.Buffer
	exitCode := int32(0)

	lines := bytes.Split(body, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		// Look for data events
		if dataEvent, ok := event["data"].(map[string]interface{}); ok {
			if stdoutData, ok := dataEvent["stdout"].(string); ok {
				stdout.WriteString(stdoutData)
			}
			if stderrData, ok := dataEvent["stderr"].(string); ok {
				stderr.WriteString(stderrData)
			}
		}

		// Look for end events
		if endEvent, ok := event["end"].(map[string]interface{}); ok {
			if exitCodeFloat, ok := endEvent["exit_code"].(float64); ok {
				exitCode = int32(exitCodeFloat)
			}
		}
	}

	return &provider.CommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// ReadFile reads a file from the sandbox via cat command.
func (p *FCProvider) ReadFile(ctx context.Context, sandboxID string, path string) ([]byte, error) {
	// Use cat command to read file content
	result, err := p.RunCommand(ctx, sandboxID, &provider.CommandConfig{
		Command: "cat",
		Args:    []string{path},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("failed to read file: %s", result.Stderr)
	}

	return []byte(result.Stdout), nil
}

// WriteFile writes content to a file in the sandbox using base64 encoding.
// This approach handles binary content and special characters correctly.
func (p *FCProvider) WriteFile(ctx context.Context, sandboxID string, path string, content []byte) error {
	// Use base64 encoding to safely transfer content
	// This avoids issues with shell escaping and special characters
	encoded := base64.StdEncoding.EncodeToString(content)

	result, err := p.RunCommand(ctx, sandboxID, &provider.CommandConfig{
		Command: "bash",
		Args:    []string{"-c", fmt.Sprintf("echo '%s' | base64 -d > %s", encoded, path)},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return err
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("failed to write file: %s", result.Stderr)
	}

	return nil
}

// ListDir lists the contents of a directory in the sandbox.
func (p *FCProvider) ListDir(ctx context.Context, sandboxID string, path string) ([]*provider.FileInfo, error) {
	// Use ls -la to list directory contents
	result, err := p.RunCommand(ctx, sandboxID, &provider.CommandConfig{
		Command: "ls",
		Args:    []string{"-la", path},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("failed to list directory: %s", result.Stderr)
	}

	// Parse ls -la output
	files := parseLsOutput(result.Stdout)
	return files, nil
}

// parseLsOutput parses the output of ls -la into FileInfo structs.
func parseLsOutput(output string) []*provider.FileInfo {
	var files []*provider.FileInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		// Skip header line and empty lines
		if strings.HasPrefix(line, "total") || strings.TrimSpace(line) == "" {
			continue
		}

		// Parse ls -la format: drwxr-xr-x 2 user group size month day time name
		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue
		}

		modeStr := parts[0]
		isDir := modeStr[0] == 'd'

		name := strings.Join(parts[7:], " ")
		// Handle symlinks
		if strings.Contains(name, " -> ") {
			name = strings.Split(name, " -> ")[0]
		}

		size := int64(0)
		if !isDir && len(parts) >= 5 {
			fmt.Sscanf(parts[4], "%d", &size)
		}

		files = append(files, &provider.FileInfo{
			Path:  name,
			Name:  name,
			IsDir: isDir,
			Size:  size,
			Mode:  0, // Would need stat command for full mode
		})
	}

	return files
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
