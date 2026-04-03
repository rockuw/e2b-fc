package fcprovider

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/e2b-dev/infra/packages/api/internal/provider"
)

// TestFCProvider_New tests creating FC provider from env
func TestFCProvider_New(t *testing.T) {
	if os.Getenv("FC_ACCESS_KEY_ID") == "" {
		t.Skip("FC_ACCESS_KEY_ID not set, skipping real FC test")
	}

	config := ConfigFromEnv()
	p, err := New(config)
	require.NoError(t, err, "Failed to create FC provider")
	assert.NotNil(t, p.client)
	assert.NotNil(t, p.config)
	t.Logf("Created FC provider for account: %s, region: %s", config.AccountID, config.Region)
}

// TestFCProvider_ListSessions tests listing all sessions (requires real FC credentials)
func TestFCProvider_ListSessions(t *testing.T) {
	if os.Getenv("FC_ACCESS_KEY_ID") == "" {
		t.Skip("FC_ACCESS_KEY_ID not set, skipping real FC test")
	}

	config := ConfigFromEnv()
	p, err := New(config)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := p.List(ctx, &provider.ListFilter{Limit: 100})
	require.NoError(t, err)

	t.Logf("Found %d sessions", len(result.Sandboxes))
	for _, s := range result.Sandboxes {
		t.Logf("  - %s: %s (template: %s)", s.SandboxID, s.State, s.TemplateID)
	}
}

// TestFCProvider_CreateSession tests creating a session (requires FC function)
// Note: This test requires a valid FC function to exist
func TestFCProvider_CreateSession(t *testing.T) {
	if os.Getenv("FC_ACCESS_KEY_ID") == "" {
		t.Skip("FC_ACCESS_KEY_ID not set, skipping real FC test")
	}

	// Get function name from env or use default
	functionName := os.Getenv("FC_TEST_FUNCTION")
	if functionName == "" {
		t.Skip("FC_TEST_FUNCTION not set, skipping create session test")
	}

	config := ConfigFromEnv()
	p, err := New(config)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a session
	createConfig := &provider.SandboxConfig{
		TemplateID: functionName,
		Timeout:    5 * time.Minute,
	}

	info, err := p.Create(ctx, createConfig)
	require.NoError(t, err, "Failed to create session")
	assert.NotEmpty(t, info.SandboxID)
	t.Logf("Created session: %s", info.SandboxID)

	// Cleanup
	defer func() {
		err := p.Delete(ctx, info.SandboxID)
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		} else {
			t.Logf("Deleted session: %s", info.SandboxID)
		}
	}()

	// Get session
	t.Run("GetSession", func(t *testing.T) {
		got, err := p.Get(ctx, info.SandboxID)
		require.NoError(t, err)
		assert.Equal(t, info.SandboxID, got.SandboxID)
		t.Logf("Got session state: %s", got.State)
	})

	// Connect
	t.Run("Connect", func(t *testing.T) {
		conn, err := p.Connect(ctx, info.SandboxID)
		require.NoError(t, err)
		assert.NotEmpty(t, conn.Endpoint)
		t.Logf("Connection endpoint: %s", conn.Endpoint)
	})
}