// Package handlers contains HTTP handlers for the API.
package handlers

import (
	"context"
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
	ctx := c.Request.Context()

	var req api.PostSandboxesJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Error{
			Message: ptr(fmt.Sprintf("Invalid request: %s", err)),
		})
		return
	}

	telemetry.ReportEvent(ctx, "Parsed create sandbox request")

	// Convert request to provider config
	timeout := 3600 // Default 1 hour
	if req.Timeout != nil {
		timeout = int(*req.Timeout)
	}

	config := &provider.SandboxConfig{
		TemplateID: req.TemplateId,
		Timeout:    time.Duration(timeout) * time.Second,
		Metadata:   req.Metadata,
		EnvVars:    req.EnvVars,
	}

	// Create sandbox via provider
	info, err := h.provider.Create(ctx, config)
	if err != nil {
		telemetry.ReportError(ctx, "failed to create sandbox", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Message: ptr(fmt.Sprintf("Failed to create sandbox: %s", err)),
		})
		return
	}

	telemetry.ReportEvent(ctx, "Created sandbox")

	// Convert to API response
	response := api.Sandbox{
		SandboxId:  &info.SandboxID,
		TemplateId: &info.TemplateID,
		Status:     ptr(string(info.State)),
		CreatedAt:  ptr(info.CreatedAt.Format(time.RFC3339)),
		ExpiresAt:  ptr(info.ExpiresAt.Format(time.RFC3339)),
		Metadata:   info.Metadata,
	}

	c.JSON(http.StatusCreated, response)
}

// GetSandboxes lists sandboxes.
// GET /sandboxes
func (h *SandboxHandlers) GetSandboxes(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse query parameters
	var params api.GetSandboxesParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, api.Error{
			Message: ptr(fmt.Sprintf("Invalid query: %s", err)),
		})
		return
	}

	// Build filter
	limit := int32(10)
	if params.Limit != nil {
		limit = *params.Limit
	}

	filter := &provider.ListFilter{
		Limit:     limit,
		NextToken: "",
	}

	if params.TemplateId != nil {
		filter.TemplateID = *params.TemplateId
	}

	// List sandboxes via provider
	result, err := h.provider.List(ctx, filter)
	if err != nil {
		telemetry.ReportError(ctx, "failed to list sandboxes", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Message: ptr(fmt.Sprintf("Failed to list sandboxes: %s", err)),
		})
		return
	}

	// Convert to API response
	sandboxes := make([]api.Sandbox, 0, len(result.Sandboxes))
	for _, info := range result.Sandboxes {
		sandboxes = append(sandboxes, api.Sandbox{
			SandboxId:  &info.SandboxID,
			TemplateId: &info.TemplateID,
			Status:     ptr(string(info.State)),
			CreatedAt:  ptr(info.CreatedAt.Format(time.RFC3339)),
			ExpiresAt:  ptr(info.ExpiresAt.Format(time.RFC3339)),
			Metadata:   info.Metadata,
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
		if err == provider.ErrSandboxNotFound {
			c.JSON(http.StatusNotFound, api.Error{
				Message: ptr("Sandbox not found"),
			})
			return
		}
		telemetry.ReportError(ctx, "failed to get sandbox", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Message: ptr(fmt.Sprintf("Failed to get sandbox: %s", err)),
		})
		return
	}

	// Convert to API response
	response := api.Sandbox{
		SandboxId:  &info.SandboxID,
		TemplateId: &info.TemplateID,
		Status:     ptr(string(info.State)),
		CreatedAt:  ptr(info.CreatedAt.Format(time.RFC3339)),
		ExpiresAt:  ptr(info.ExpiresAt.Format(time.RFC3339)),
		Metadata:   info.Metadata,
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
		if err == provider.ErrSandboxNotFound {
			c.JSON(http.StatusNotFound, api.Error{
				Message: ptr("Sandbox not found"),
			})
			return
		}
		telemetry.ReportError(ctx, "failed to delete sandbox", err)
		c.JSON(http.StatusInternalServerError, api.Error{
			Message: ptr(fmt.Sprintf("Failed to delete sandbox: %s", err)),
		})
		return
	}

	telemetry.ReportEvent(ctx, "Deleted sandbox")

	c.Status(http.StatusNoContent)
}

// Helper functions

func ptr[T any](v T) *T {
	return &v
}