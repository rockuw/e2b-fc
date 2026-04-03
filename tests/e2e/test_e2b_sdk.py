#!/usr/bin/env python3
"""
E2E tests for E2B-FC service using vanilla e2b SDK.

These tests validate API compatibility with the standard e2b SDK.
The tests require:
- E2B_API_KEY environment variable (or skip tests)
- Running E2B-FC service (default: http://localhost:8080)
"""

import os
import time
import pytest

# Use vanilla e2b SDK
from e2b import Sandbox


# Configure test base URL
E2BFC_BASE_URL = os.environ.get("E2BFC_BASE_URL", "http://localhost:8080")


@pytest.fixture(scope="module")
def api_key():
    """Get API key from environment."""
    key = os.environ.get("E2B_API_KEY")
    if not key:
        pytest.skip("E2B_API_KEY not set")
    return key


@pytest.fixture(scope="module")
def sandbox_client(api_key):
    """Create sandbox client configured for E2B-FC."""
    # Configure the client to use our E2B-FC endpoint
    # The e2b SDK uses E2B_API_KEY for authentication
    yield Sandbox


class TestSandboxCRUD:
    """Test sandbox CRUD operations using vanilla e2b SDK."""

    def test_create_sandbox(self, sandbox_client):
        """
        Test: Create a sandbox and get sandbox ID.

        Acceptance Criteria:
        - Can create a sandbox with template
        - Returns a valid sandbox ID
        """
        sandbox = sandbox_client.create(
            template="code-interpreter-v1",
            timeout=300  # 5 minutes
        )

        assert sandbox is not None
        assert sandbox.sandbox_id is not None
        assert len(sandbox.sandbox_id) > 0

        # Cleanup
        sandbox.kill()

    def test_list_sandboxes(self, sandbox_client):
        """
        Test: List sandboxes with pagination.

        Acceptance Criteria:
        - Can list sandboxes
        - Returns list with pagination support
        """
        # Create a sandbox first
        sandbox = sandbox_client.create(template="code-interpreter-v1")

        try:
            # List sandboxes
            sandboxes = sandbox_client.list()

            assert sandboxes is not None
            assert isinstance(sandboxes, list)

            # Our sandbox should be in the list
            sandbox_ids = [s.sandbox_id for s in sandboxes]
            assert sandbox.sandbox_id in sandbox_ids
        finally:
            sandbox.kill()

    def test_get_sandbox_info(self, sandbox_client):
        """
        Test: Get sandbox info by ID.

        Acceptance Criteria:
        - Can get sandbox info by ID
        - Returns correct state
        """
        sandbox = sandbox_client.create(template="code-interpreter-v1")

        try:
            # Get sandbox info
            info = sandbox.get_info()

            assert info is not None
            assert info.sandbox_id == sandbox.sandbox_id
            assert info.status in ["running", "idle"]
        finally:
            sandbox.kill()

    def test_kill_sandbox(self, sandbox_client):
        """
        Test: Kill a sandbox.

        Acceptance Criteria:
        - Can kill a sandbox
        - Sandbox is removed
        """
        sandbox = sandbox_client.create(template="code-interpreter-v1")
        sandbox_id = sandbox.sandbox_id

        # Kill the sandbox
        sandbox.kill()

        # Verify it's deleted
        time.sleep(1)  # Wait for deletion to propagate
        sandboxes = sandbox_client.list()
        sandbox_ids = [s.sandbox_id for s in sandboxes]

        assert sandbox_id not in sandbox_ids

    def test_sandbox_auto_expire(self, sandbox_client):
        """
        Test: Sandbox auto-expires after TTL.

        Acceptance Criteria:
        - Sandbox expires after timeout
        - No longer accessible after expiration

        Note: This test is slow as it waits for expiration.
        """
        # Create sandbox with very short timeout (10 seconds)
        sandbox = sandbox_client.create(
            template="code-interpreter-v1",
            timeout=10
        )

        sandbox_id = sandbox.sandbox_id

        # Wait for expiration (with buffer)
        time.sleep(15)

        # Sandbox should be expired
        sandboxes = sandbox_client.list()
        sandbox_ids = [s.sandbox_id for s in sandboxes]

        assert sandbox_id not in sandbox_ids


class TestSandboxFileSystem:
    """Test file system operations (Phase 2)."""

    @pytest.mark.skip(reason="Phase 2 feature")
    def test_write_and_read_file(self, sandbox_client):
        """Test writing and reading files."""
        pass

    @pytest.mark.skip(reason="Phase 2 feature")
    def test_file_exists(self, sandbox_client):
        """Test checking if file exists."""
        pass

    @pytest.mark.skip(reason="Phase 2 feature")
    def test_list_directory(self, sandbox_client):
        """Test listing directory contents."""
        pass


class TestSandboxCommands:
    """Test terminal commands (Phase 2)."""

    @pytest.mark.skip(reason="Phase 2 feature")
    def test_run_command_sync(self, sandbox_client):
        """Test synchronous command execution."""
        pass

    @pytest.mark.skip(reason="Phase 2 feature")
    def test_run_command_streaming(self, sandbox_client):
        """Test streaming command output."""
        pass


class TestCodeExecution:
    """Test code execution (Phase 3)."""

    @pytest.mark.skip(reason="Phase 3 feature")
    def test_run_python_code(self, sandbox_client):
        """Test running Python code."""
        pass

    @pytest.mark.skip(reason="Phase 3 feature")
    def test_run_javascript_code(self, sandbox_client):
        """Test running JavaScript code."""
        pass


if __name__ == "__main__":
    pytest.main([__file__, "-v"])