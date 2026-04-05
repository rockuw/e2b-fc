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

# Configure E2B SDK to use custom API URL before importing
E2BFC_BASE_URL = os.environ.get("E2BFC_BASE_URL", "http://localhost:8081")
os.environ["E2B_API_URL"] = E2BFC_BASE_URL

# Use vanilla e2b SDK
from e2b import Sandbox


# Template to use for tests (maps to FC function name)
TEST_TEMPLATE = os.environ.get("E2B_TEST_TEMPLATE", "test-sandbox")


@pytest.fixture(scope="module")
def api_key():
    """Get API key from environment."""
    key = os.environ.get("E2B_API_KEY")
    if not key:
        # Use a dummy key for local testing
        key = "test-api-key"
    return key


@pytest.fixture(scope="module")
def sandbox_client(api_key):
    """Create sandbox client configured for E2B-FC."""
    # Configure the client to use our E2B-FC endpoint
    # The e2b SDK uses E2B_API_URL from environment
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
            template=TEST_TEMPLATE,
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
        sandbox = sandbox_client.create(template=TEST_TEMPLATE)

        try:
            # List sandboxes - returns a paginator
            paginator = sandbox_client.list()

            # Get items from paginator
            sandbox_list = paginator.next_items()

            assert sandbox_list is not None

            # Our sandbox should be in the list
            sandbox_ids = [s.sandbox_id for s in sandbox_list]
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
        sandbox = sandbox_client.create(template=TEST_TEMPLATE)

        try:
            # Get sandbox info
            info = sandbox.get_info()

            assert info is not None
            assert info.sandbox_id == sandbox.sandbox_id
            # state is a SandboxState enum
            assert info.state.value in ["running", "paused"]
        finally:
            sandbox.kill()

    def test_kill_sandbox(self, sandbox_client):
        """
        Test: Kill a sandbox.

        Acceptance Criteria:
        - Can kill a sandbox
        - Sandbox is removed
        """
        sandbox = sandbox_client.create(template=TEST_TEMPLATE)
        sandbox_id = sandbox.sandbox_id

        # Kill the sandbox
        sandbox.kill()

        # Verify it's deleted
        time.sleep(1)  # Wait for deletion to propagate
        paginator = sandbox_client.list()
        sandbox_list = paginator.next_items()
        sandbox_ids = [s.sandbox_id for s in sandbox_list]

        assert sandbox_id not in sandbox_ids

    def test_sandbox_auto_expire(self, sandbox_client):
        """
        Test: Sandbox auto-expires after TTL.

        Acceptance Criteria:
        - Sandbox expires after timeout
        - No longer accessible after expiration

        Note: This test is slow as it waits for expiration.
        FC requires minimum TTL of 60 seconds, so we use 60 seconds.
        """
        # Create sandbox with minimum TTL (60 seconds for FC)
        sandbox = sandbox_client.create(
            template=TEST_TEMPLATE,
            timeout=60  # FC minimum is 60 seconds
        )

        sandbox_id = sandbox.sandbox_id

        # Wait for expiration (with buffer)
        time.sleep(70)

        # Sandbox should be expired
        paginator = sandbox_client.list()
        sandbox_list = paginator.next_items()
        sandbox_ids = [s.sandbox_id for s in sandbox_list]

        assert sandbox_id not in sandbox_ids


class TestSandboxFileSystem:
    """Test file system operations (Phase 2)."""

    def test_write_and_read_file(self, sandbox_client):
        """
        Test: Write file and read it back.

        Acceptance Criteria:
        - Can write file to the sandbox
        - Can read file from the sandbox
        - Content matches what was written
        """
        sandbox = sandbox_client.create(template=TEST_TEMPLATE, timeout=300)

        try:
            # Write file
            test_content = "Hello, World!"
            sandbox.files.write("/workspace/test.txt", test_content)

            # Read it back
            content = sandbox.files.read("/workspace/test.txt")
            assert content == test_content
        finally:
            sandbox.kill()

    def test_file_exists(self, sandbox_client):
        """
        Test: Check if file exists.

        Acceptance Criteria:
        - Can check if a file exists
        - Returns True for existing files
        - Returns False for non-existent files
        """
        sandbox = sandbox_client.create(template=TEST_TEMPLATE, timeout=300)

        try:
            # Create a file
            sandbox.files.write("/workspace/exists_test.txt", "test")

            # Check exists
            assert sandbox.files.exists("/workspace/exists_test.txt") is True
            assert sandbox.files.exists("/workspace/non_existent.txt") is False
        finally:
            sandbox.kill()

    def test_list_directory(self, sandbox_client):
        """
        Test: List directory contents.

        Acceptance Criteria:
        - Can list directory contents
        - Returns file metadata (name, size, type)
        """
        sandbox = sandbox_client.create(template=TEST_TEMPLATE, timeout=300)

        try:
            # Create some files
            sandbox.files.write("/workspace/file1.txt", "content1")
            sandbox.files.write("/workspace/file2.txt", "content2")

            # List directory
            entries = sandbox.files.list("/workspace")

            # Verify entries
            names = [e.name for e in entries]
            assert "file1.txt" in names
            assert "file2.txt" in names
        finally:
            sandbox.kill()


class TestSandboxCommands:
    """Test terminal commands (Phase 2)."""

    def test_run_command_sync(self, sandbox_client):
        """
        Test: Run command synchronously.

        Acceptance Criteria:
        - Can execute a command and receive stdout/stderr
        - Command output is captured correctly
        """
        sandbox = sandbox_client.create(template=TEST_TEMPLATE, timeout=300)

        try:
            # Run echo command
            result = sandbox.commands.run("echo hello")

            assert result.stdout.strip() == "hello"
            assert result.exit_code == 0
        finally:
            sandbox.kill()

    def test_run_command_with_timeout(self, sandbox_client):
        """
        Test: Command respects timeout settings.

        Acceptance Criteria:
        - Commands with timeout are killed after timeout
        - Timeout returns appropriate exit code
        """
        sandbox = sandbox_client.create(template=TEST_TEMPLATE, timeout=300)

        try:
            # Run a long-running command with short timeout
            result = sandbox.commands.run("sleep 10", timeout=2)

            # Command should be killed due to timeout
            # Note: exit_code 124 is timeout in bash, or it may have been killed by signal
            assert result.exit_code != 0
        finally:
            sandbox.kill()

    def test_list_directory_via_command(self, sandbox_client):
        """
        Test: List directory via command execution.

        Acceptance Criteria:
        - Can execute ls command and verify files are listed
        """
        sandbox = sandbox_client.create(template=TEST_TEMPLATE, timeout=300)

        try:
            # Create a file first
            sandbox.files.write("/workspace/cmd_test.txt", "test")

            # Run ls command
            result = sandbox.commands.run("ls /workspace")

            assert "cmd_test.txt" in result.stdout
        finally:
            sandbox.kill()


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