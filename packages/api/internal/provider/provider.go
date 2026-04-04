// Package provider re-exports the provider interface from the shared package.
// This allows internal code to import from the internal path while
// external packages (like fc-provider) import from the shared package.
package provider

import (
	"github.com/e2b-dev/infra/packages/shared/pkg/provider"
)

// Re-export types and interfaces
type (
	SandboxConfig    = provider.SandboxConfig
	SandboxInfo      = provider.SandboxInfo
	SandboxState     = provider.SandboxState
	ConnectionInfo   = provider.ConnectionInfo
	ListFilter       = provider.ListFilter
	ListResult       = provider.ListResult
	SandboxProvider  = provider.SandboxProvider
)

// Re-export errors
var (
	ErrSandboxNotFound      = provider.ErrSandboxNotFound
	ErrSandboxAlreadyExists = provider.ErrSandboxAlreadyExists
	ErrInvalidTemplate      = provider.ErrInvalidTemplate
	ErrProviderUnavailable  = provider.ErrProviderUnavailable
)

// Re-export constants
const (
	SandboxStateRunning SandboxState = provider.SandboxStateRunning
	SandboxStateIdle    SandboxState = provider.SandboxStateIdle
	SandboxStateExpired SandboxState = provider.SandboxStateExpired
	SandboxStateDeleted SandboxState = provider.SandboxStateDeleted
)