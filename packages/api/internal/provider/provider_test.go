package provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProvider_Create(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	config := &SandboxConfig{
		TemplateID: "code-interpreter-v1",
		Timeout:    time.Hour,
		Metadata:   map[string]string{"key": "value"},
	}

	info, err := p.Create(ctx, config)

	require.NoError(t, err)
	assert.NotEmpty(t, info.SandboxID)
	assert.Equal(t, "code-interpreter-v1", info.TemplateID)
	assert.Equal(t, SandboxStateRunning, info.State)
	assert.NotZero(t, info.CreatedAt)
	assert.True(t, info.ExpiresAt.After(info.CreatedAt))
}

func TestMockProvider_Get(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	// Create a sandbox first
	config := &SandboxConfig{
		TemplateID: "code-interpreter-v1",
		Timeout:    time.Hour,
	}
	created, err := p.Create(ctx, config)
	require.NoError(t, err)

	// Get the sandbox
	info, err := p.Get(ctx, created.SandboxID)

	require.NoError(t, err)
	assert.Equal(t, created.SandboxID, info.SandboxID)
	assert.Equal(t, created.TemplateID, info.TemplateID)
}

func TestMockProvider_Get_NotFound(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	_, err := p.Get(ctx, "non-existent")

	assert.ErrorIs(t, err, ErrSandboxNotFound)
}

func TestMockProvider_Delete(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	// Create a sandbox first
	config := &SandboxConfig{
		TemplateID: "code-interpreter-v1",
		Timeout:    time.Hour,
	}
	created, err := p.Create(ctx, config)
	require.NoError(t, err)

	// Delete the sandbox
	err = p.Delete(ctx, created.SandboxID)
	require.NoError(t, err)

	// Verify it's deleted
	_, err = p.Get(ctx, created.SandboxID)
	assert.ErrorIs(t, err, ErrSandboxNotFound)
}

func TestMockProvider_Delete_NotFound(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	err := p.Delete(ctx, "non-existent")

	assert.ErrorIs(t, err, ErrSandboxNotFound)
}

func TestMockProvider_List(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	// Create multiple sandboxes
	for range 5 {
		config := &SandboxConfig{
			TemplateID: "code-interpreter-v1",
			Timeout:    time.Hour,
		}
		_, err := p.Create(ctx, config)
		require.NoError(t, err)
	}

	// List sandboxes
	result, err := p.List(ctx, &ListFilter{Limit: 10})

	require.NoError(t, err)
	assert.Len(t, result.Sandboxes, 5)
}

func TestMockProvider_List_WithTemplateFilter(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	// Create sandboxes with different templates
	config1 := &SandboxConfig{TemplateID: "code-interpreter-v1", Timeout: time.Hour}
	config2 := &SandboxConfig{TemplateID: "browser-v1", Timeout: time.Hour}

	_, err := p.Create(ctx, config1)
	require.NoError(t, err)
	_, err = p.Create(ctx, config2)
	require.NoError(t, err)

	// List with filter
	result, err := p.List(ctx, &ListFilter{
		TemplateID: "code-interpreter-v1",
		Limit:      10,
	})

	require.NoError(t, err)
	assert.Len(t, result.Sandboxes, 1)
	assert.Equal(t, "code-interpreter-v1", result.Sandboxes[0].TemplateID)
}

func TestMockProvider_Connect(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	// Create a sandbox first
	config := &SandboxConfig{
		TemplateID: "code-interpreter-v1",
		Timeout:    time.Hour,
	}
	created, err := p.Create(ctx, config)
	require.NoError(t, err)

	// Connect to the sandbox
	connInfo, err := p.Connect(ctx, created.SandboxID)

	require.NoError(t, err)
	assert.NotEmpty(t, connInfo.Endpoint)
	assert.NotEmpty(t, connInfo.AccessToken)
	assert.Equal(t, created.SandboxID, connInfo.SessionID)
}

func TestMockProvider_Connect_NotFound(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	_, err := p.Connect(ctx, "non-existent")

	assert.ErrorIs(t, err, ErrSandboxNotFound)
}

func TestMockProvider_ErrorHandling(t *testing.T) {
	t.Parallel()
	p := NewMockProvider()
	ctx := context.Background()

	// Set errors
	p.CreateError = ErrProviderUnavailable

	// Verify error is returned
	_, err := p.Create(ctx, &SandboxConfig{})
	assert.ErrorIs(t, err, ErrProviderUnavailable)
}