// Package handlers contains HTTP handlers for the API.
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/e2b-dev/infra/packages/api/internal/api"
	"github.com/e2b-dev/infra/packages/api/internal/provider"
	"github.com/e2b-dev/infra/packages/shared/pkg/telemetry"
)

// SandboxHandlers contains handlers for sandbox operations using SandboxProvider.
type SandboxHandlers struct {
	provider provider.SandboxProvider
}

// NewSandboxHandlers creates new sandbox handlers.
func NewSandboxHandlers(p provider.SandboxProvider) *SandboxHandlers {
	return &SandboxHandlers{provider: p}
}

// PostSandboxes creates a new sandbox.
// POST /sandboxes
func (h *SandboxHandlers) PostSandboxes(c *gin.Context) {
	var req api.NewSandbox
	if err := c.ShouldBindJSON(&req); err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to parse request", err)
		c.JSON(http.StatusBadRequest, api.Error{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid request: %s", err),
		})

		return
	}

	telemetry.ReportEvent(c.Request.Context(), "Parsed create sandbox request")

	// Convert request to provider config
	timeout := int32(3600) // Default 1 hour
	if req.Timeout != nil {
		timeout = *req.Timeout
	}

	// Convert metadata and envVars from pointer types
	var metadata map[string]string
	if req.Metadata != nil {
		metadata = *req.Metadata
	}

	var envVars map[string]string
	if req.EnvVars != nil {
		envVars = *req.EnvVars
	}

	config := &provider.SandboxConfig{
		TemplateID: req.TemplateID,
		Timeout:    time.Duration(timeout) * time.Second,
		Metadata:   metadata,
		EnvVars:    envVars,
	}

	// Create sandbox via provider
	info, err := h.provider.Create(c.Request.Context(), config)
	if err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to create sandbox", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to create sandbox: %s", err),
		})

		return
	}

	telemetry.ReportEvent(c.Request.Context(), "Created sandbox")

	// Get connection info for access tokens
	connInfo, err := h.provider.Connect(c.Request.Context(), info.SandboxID)
	if err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to get connection info", err)
		// Still return the sandbox, but without access tokens
		connInfo = &provider.ConnectionInfo{}
	}

	// Convert to API response
	envdVersion := api.EnvdVersion("1.0.0")
	response := api.Sandbox{
		SandboxID:          info.SandboxID,
		TemplateID:         info.TemplateID,
		ClientID:           info.SandboxID, // Use sandbox ID as client ID for FC
		EnvdVersion:        envdVersion,
		EnvdAccessToken:    &connInfo.AccessToken,
		TrafficAccessToken: &connInfo.AccessToken,
	}

	c.JSON(http.StatusCreated, response)
}

// PostV2Sandboxes creates a new sandbox for v2 API (e2b SDK).
// POST /v2/sandboxes
func (h *SandboxHandlers) PostV2Sandboxes(c *gin.Context) {
	var req api.NewSandbox
	if err := c.ShouldBindJSON(&req); err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to parse request", err)
		c.JSON(http.StatusBadRequest, api.Error{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid request: %s", err),
		})

		return
	}

	telemetry.ReportEvent(c.Request.Context(), "Parsed create sandbox request")

	// Convert request to provider config
	timeout := int32(3600) // Default 1 hour
	if req.Timeout != nil {
		timeout = *req.Timeout
	}

	// Convert metadata and envVars from pointer types
	var metadata map[string]string
	if req.Metadata != nil {
		metadata = *req.Metadata
	}

	var envVars map[string]string
	if req.EnvVars != nil {
		envVars = *req.EnvVars
	}

	config := &provider.SandboxConfig{
		TemplateID: req.TemplateID,
		Timeout:    time.Duration(timeout) * time.Second,
		Metadata:   metadata,
		EnvVars:    envVars,
	}

	// Create sandbox via provider
	info, err := h.provider.Create(c.Request.Context(), config)
	if err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to create sandbox", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to create sandbox: %s", err),
		})

		return
	}

	telemetry.ReportEvent(c.Request.Context(), "Created sandbox")

	// Get connection info for access tokens
	connInfo, err := h.provider.Connect(c.Request.Context(), info.SandboxID)
	if err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to get connection info", err)
		// Still return the sandbox, but without access tokens
		connInfo = &provider.ConnectionInfo{}
	}

	// Convert to API response - v2 format
	envdVersion := api.EnvdVersion("1.0.0")
	response := api.Sandbox{
		SandboxID:          info.SandboxID,
		TemplateID:         info.TemplateID,
		ClientID:           info.SandboxID, // Use sandbox ID as client ID for FC
		EnvdVersion:        envdVersion,
		EnvdAccessToken:    &connInfo.AccessToken,
		TrafficAccessToken: &connInfo.AccessToken,
	}

	c.JSON(http.StatusCreated, response)
}

// GetV2Sandboxes lists sandboxes for v2 API (e2b SDK).
// GET /v2/sandboxes
func (h *SandboxHandlers) GetV2Sandboxes(c *gin.Context) {
	// Build filter
	filter := &provider.ListFilter{
		Limit:     100,
		NextToken: "",
	}

	// List sandboxes via provider
	result, err := h.provider.List(c.Request.Context(), filter)
	if err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to list sandboxes", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to list sandboxes: %s", err),
		})

		return
	}

	// Convert to API response with full details
	sandboxes := make([]api.ListedSandbox, 0, len(result.Sandboxes))
	for _, info := range result.Sandboxes {
		sandbox := api.ListedSandbox{
			SandboxID:   info.SandboxID,
			TemplateID:  info.TemplateID,
			ClientID:    info.SandboxID,
			CpuCount:    1,
			DiskSizeMB:  512,
			MemoryMB:    512,
			EnvdVersion: "1.0.0",
			StartedAt:   info.CreatedAt,
			EndAt:       info.ExpiresAt,
		}
		// Map state
		switch info.State {
		case provider.SandboxStateRunning:
			sandbox.State = "running"
		case provider.SandboxStateIdle:
			sandbox.State = "idle"
		case provider.SandboxStateExpired:
			sandbox.State = "expired"
		default:
			sandbox.State = "running"
		}
		sandboxes = append(sandboxes, sandbox)
	}

	c.JSON(http.StatusOK, sandboxes)
}

// GetSandboxes lists sandboxes.
// GET /sandboxes
func (h *SandboxHandlers) GetSandboxes(c *gin.Context) {
	// Parse query parameters
	var params api.GetSandboxesParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, api.Error{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid query: %s", err),
		})

		return
	}

	// Build filter - use default limit since API doesn't support pagination params
	filter := &provider.ListFilter{
		Limit:     100, // Default limit
		NextToken: "",
	}

	// Note: Metadata filtering is not yet implemented at the provider level
	_ = params.Metadata

	// List sandboxes via provider
	result, err := h.provider.List(c.Request.Context(), filter)
	if err != nil {
		telemetry.ReportError(c.Request.Context(), "failed to list sandboxes", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to list sandboxes: %s", err),
		})

		return
	}

	// Convert to API response
	sandboxes := make([]api.Sandbox, 0, len(result.Sandboxes))
	for _, info := range result.Sandboxes {
		sandboxes = append(sandboxes, api.Sandbox{
			SandboxID:  info.SandboxID,
			TemplateID: info.TemplateID,
			ClientID:   info.SandboxID,
		})
	}

	c.JSON(http.StatusOK, sandboxes)
}

// GetSandboxesSandboxID gets a sandbox by ID.
// GET /sandboxes/{sandboxID}
func (h *SandboxHandlers) GetSandboxesSandboxID(c *gin.Context, sandboxID string) {
	ctx := c.Request.Context()

	// Get sandbox via provider
	info, err := h.provider.Get(ctx, sandboxID)
	if err != nil {
		if errors.Is(err, provider.ErrSandboxNotFound) {
			c.JSON(http.StatusNotFound, api.Error{
				Code:    http.StatusNotFound,
				Message: "Sandbox not found",
			})

			return
		}
		telemetry.ReportError(ctx, "failed to get sandbox", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to get sandbox: %s", err),
		})

		return
	}

	// Get connection info
	connInfo, err := h.provider.Connect(ctx, info.SandboxID)
	if err != nil {
		connInfo = &provider.ConnectionInfo{}
	}

	// Convert to API response with full details
	state := "running"
	switch info.State {
	case provider.SandboxStateRunning:
		state = "running"
	case provider.SandboxStateIdle:
		state = "idle"
	case provider.SandboxStateExpired:
		state = "expired"
	}

	response := api.SandboxDetail{
		SandboxID:       info.SandboxID,
		TemplateID:      info.TemplateID,
		ClientID:        info.SandboxID,
		CpuCount:        1,
		DiskSizeMB:      512,
		MemoryMB:        512,
		EnvdVersion:     "1.0.0",
		EnvdAccessToken: &connInfo.AccessToken,
		StartedAt:       info.CreatedAt,
		EndAt:           info.ExpiresAt,
		State:           api.SandboxState(state),
	}

	c.JSON(http.StatusOK, response)
}

// DeleteSandboxesSandboxID deletes a sandbox.
// DELETE /sandboxes/{sandboxID}
func (h *SandboxHandlers) DeleteSandboxesSandboxID(c *gin.Context, sandboxID string) {
	ctx := c.Request.Context()

	// Delete sandbox via provider
	err := h.provider.Delete(ctx, sandboxID)
	if err != nil {
		if errors.Is(err, provider.ErrSandboxNotFound) {
			c.JSON(http.StatusNotFound, api.Error{
				Code:    http.StatusNotFound,
				Message: "Sandbox not found",
			})

			return
		}
		telemetry.ReportError(ctx, "failed to delete sandbox", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to delete sandbox: %s", err),
		})

		return
	}

	telemetry.ReportEvent(ctx, "Deleted sandbox")

	c.Status(http.StatusNoContent)
}

// CommandRequest represents a request to run a command in a sandbox.
type CommandRequest struct {
	// Cmd is the command to execute
	Cmd string `binding:"required" json:"cmd"`
	// Args are the command arguments
	Args []string `json:"args"`
	// EnvVars are environment variables for the command
	EnvVars map[string]string `json:"env_vars"`
	// Cwd is the working directory
	Cwd string `json:"cwd"`
	// Timeout is the command timeout in seconds
	Timeout int32 `json:"timeout"`
}

// CommandResponse represents the response from running a command.
type CommandResponse struct {
	ExitCode int32  `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// PostSandboxesSandboxIDCommands runs a command in a sandbox.
// POST /sandboxes/{sandboxID}/commands
func (h *SandboxHandlers) PostSandboxesSandboxIDCommands(c *gin.Context, sandboxID string) {
	ctx := c.Request.Context()

	// Parse request
	var req CommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Error{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid request: %s", err),
		})

		return
	}

	// Build command config
	timeout := time.Duration(req.Timeout) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second // Default 60 seconds
	}

	config := &provider.CommandConfig{
		Command: req.Cmd,
		Args:    req.Args,
		EnvVars: req.EnvVars,
		Cwd:     req.Cwd,
		Timeout: timeout,
	}

	// Run command
	result, err := h.provider.RunCommand(ctx, sandboxID, config)
	if err != nil {
		if errors.Is(err, provider.ErrSandboxNotFound) {
			c.JSON(http.StatusNotFound, api.Error{
				Code:    http.StatusNotFound,
				Message: "Sandbox not found",
			})

			return
		}
		telemetry.ReportError(ctx, "failed to run command", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to run command: %s", err),
		})

		return
	}

	// Return result
	response := CommandResponse{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}

	c.JSON(http.StatusOK, response)
}

// FileRequest represents a request to read/write a file.
type FileRequest struct {
	// Path is the file path
	Path string `binding:"required" json:"path"`
	// Content is the file content (for write)
	Content string `json:"content"`
}

// FileResponse represents the response for file operations.
type FileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
}

// GetSandboxesSandboxIDFiles reads a file from a sandbox.
// GET /sandboxes/{sandboxID}/files?path=/workspace/file.txt
func (h *SandboxHandlers) GetSandboxesSandboxIDFiles(c *gin.Context, sandboxID string) {
	ctx := c.Request.Context()

	// Get file path from query
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, api.Error{
			Code:    http.StatusBadRequest,
			Message: "path query parameter is required",
		})

		return
	}

	// Read file
	content, err := h.provider.ReadFile(ctx, sandboxID, path)
	if err != nil {
		if errors.Is(err, provider.ErrSandboxNotFound) {
			c.JSON(http.StatusNotFound, api.Error{
				Code:    http.StatusNotFound,
				Message: "Sandbox not found",
			})

			return
		}

		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to read file: %s", err),
		})

		return
	}

	response := FileResponse{
		Path:    path,
		Content: string(content),
	}

	c.JSON(http.StatusOK, response)
}

// PostSandboxesSandboxIDFiles writes a file to a sandbox.
// POST /sandboxes/{sandboxID}/files
func (h *SandboxHandlers) PostSandboxesSandboxIDFiles(c *gin.Context, sandboxID string) {
	ctx := c.Request.Context()

	// Parse request
	var req FileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Error{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid request: %s", err),
		})

		return
	}

	// Write file
	err := h.provider.WriteFile(ctx, sandboxID, req.Path, []byte(req.Content))
	if err != nil {
		if errors.Is(err, provider.ErrSandboxNotFound) {
			c.JSON(http.StatusNotFound, api.Error{
				Code:    http.StatusNotFound,
				Message: "Sandbox not found",
			})

			return
		}

		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to write file: %s", err),
		})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path": req.Path,
		"size": len(req.Content),
	})
}

// GetSandboxesSandboxIDDir lists directory contents.
// GET /sandboxes/{sandboxID}/dir?path=/workspace
func (h *SandboxHandlers) GetSandboxesSandboxIDDir(c *gin.Context, sandboxID string) {
	ctx := c.Request.Context()

	// Get directory path from query
	path := c.Query("path")
	if path == "" {
		path = "/workspace"
	}

	// List directory
	files, err := h.provider.ListDir(ctx, sandboxID, path)
	if err != nil {
		if errors.Is(err, provider.ErrSandboxNotFound) {
			c.JSON(http.StatusNotFound, api.Error{
				Code:    http.StatusNotFound,
				Message: "Sandbox not found",
			})

			return
		}

		c.JSON(http.StatusInternalServerError, api.Error{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to list directory: %s", err),
		})

		return
	}

	// Convert to response
	response := make([]FileResponse, 0, len(files))
	for _, f := range files {
		response = append(response, FileResponse{
			Path:  f.Path,
			IsDir: f.IsDir,
			Size:  f.Size,
		})
	}

	c.JSON(http.StatusOK, response)
}
