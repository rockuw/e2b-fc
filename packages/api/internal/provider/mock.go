// Package provider provides a mock implementation of SandboxProvider for testing.
package provider

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MockProvider is a mock implementation of SandboxProvider for testing.
type MockProvider struct {
	mu        sync.RWMutex
	sandboxes map[string]*SandboxInfo

	// CreateError can be set to return an error on Create
	CreateError error

	// GetError can be set to return an error on Get
	GetError error

	// DeleteError can be set to return an error on Delete
	DeleteError error

	// ListError can be set to return an error on List
	ListError error

	// ConnectError can be set to return an error on Connect
	ConnectError error
}

// NewMockProvider creates a new mock provider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		sandboxes: make(map[string]*SandboxInfo),
	}
}

// Create creates a new sandbox in memory.
func (m *MockProvider) Create(ctx context.Context, config *SandboxConfig) (*SandboxInfo, error) {
	if m.CreateError != nil {
		return nil, m.CreateError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sandboxID := uuid.New().String()
	now := time.Now()

	info := &SandboxInfo{
		SandboxID:  sandboxID,
		TemplateID: config.TemplateID,
		State:      SandboxStateRunning,
		CreatedAt:  now,
		ExpiresAt:  now.Add(config.Timeout),
		Metadata:   config.Metadata,
	}

	m.sandboxes[sandboxID] = info

	return info, nil
}

// Get returns a sandbox by ID.
func (m *MockProvider) Get(ctx context.Context, sandboxID string) (*SandboxInfo, error) {
	if m.GetError != nil {
		return nil, m.GetError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok := m.sandboxes[sandboxID]
	if !ok {
		return nil, ErrSandboxNotFound
	}

	return info, nil
}

// Delete removes a sandbox by ID.
func (m *MockProvider) Delete(ctx context.Context, sandboxID string) error {
	if m.DeleteError != nil {
		return m.DeleteError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sandboxes[sandboxID]; !ok {
		return ErrSandboxNotFound
	}

	delete(m.sandboxes, sandboxID)
	return nil
}

// List returns a paginated list of sandboxes.
func (m *MockProvider) List(ctx context.Context, filter *ListFilter) (*ListResult, error) {
	if m.ListError != nil {
		return nil, m.ListError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	limit := int(filter.Limit)
	if limit <= 0 {
		limit = 10
	}

	var sandboxes []*SandboxInfo
	for _, info := range m.sandboxes {
		if filter.TemplateID != "" && info.TemplateID != filter.TemplateID {
			continue
		}
		sandboxes = append(sandboxes, info)
	}

	// Simple pagination - just return up to limit
	result := &ListResult{}
	if len(sandboxes) > limit {
		result.Sandboxes = sandboxes[:limit]
		result.NextToken = "next"
	} else {
		result.Sandboxes = sandboxes
	}

	return result, nil
}

// Connect returns connection info for a sandbox.
func (m *MockProvider) Connect(ctx context.Context, sandboxID string) (*ConnectionInfo, error) {
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.sandboxes[sandboxID]; !ok {
		return nil, ErrSandboxNotFound
	}

	return &ConnectionInfo{
		Endpoint:     "http://localhost:49983",
		AccessToken:  "test-token",
		SessionID:    sandboxID,
	}, nil
}

// Count returns the number of sandboxes.
func (m *MockProvider) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sandboxes)
}

// Reset clears all sandboxes.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sandboxes = make(map[string]*SandboxInfo)
	m.CreateError = nil
	m.GetError = nil
	m.DeleteError = nil
	m.ListError = nil
	m.ConnectError = nil
}