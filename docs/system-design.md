# E2B-FC System Design

## Overview

This document describes the system design for an e2b-compatible service built on Aliyun FunctionCompute (FC) Session feature. The service provides sandbox environments for AI code execution, leveraging FC's session affinity and isolation modes.

## Goals

1. Build an e2b-compatible API server using the existing e2b-dev/infra codebase
2. Replace Firecracker-based sandbox implementation with Aliyun FC Session API
3. Implement core features from Tencent Cloud Agent Runtime (excluding mobile operations)

## Core Features to Implement

Based on the Tencent Cloud documentation, the following features are required:

### 1. Sandbox Instance Management
- `create` - Create a sandbox instance with template and timeout
- `list` - List sandbox instances with pagination
- `get_info` - Get sandbox instance information
- `kill` - Delete/kill a sandbox instance

### 2. File System Operations
- `read_file` - Read file content
- `write_file` - Write file content
- `upload_file` - Upload file to sandbox
- `exists` - Check if file/directory exists
- `rename` / `move` - Rename/move files
- `make_dir` - Create directory
- `watch_dir` - Watch directory for changes
- `list_dir` - List directory contents
- `remove` - Remove files/directories

### 3. Terminal Commands
- `run` - Execute terminal command (sync/stream/background modes)
- `send_stdin` - Send stdin input to running process
- `list` - List running background commands
- `kill` - Kill a running command

### 4. Code Execution
- `run_code` - Execute code in Jupyter kernel
- Support multiple languages: Python, JavaScript, TypeScript, Java, R, Bash
- `create_code_context` - Create execution context
- Streaming output support
- Environment variables and timeout support

### 5. Browser Operations
- `live_url` - URL for viewing browser interface
- `cdp_url` - Chrome DevTools Protocol URL for Playwright control

---

## Architecture

### High-Level Architecture

```
                                    ┌─────────────────────────────────────────┐
                                    │           Aliyun Cloud                  │
                                    │  ┌───────────────────────────────────┐  │
                                    │  │      FunctionCompute (FC)         │  │
                                    │  │  ┌─────────────────────────────┐  │  │
                                    │  │  │   FC Session (Isolation)    │  │  │
                                    │  │  │  ┌───────────────────────┐  │  │  │
                                    │  │  │  │   Sandbox Function    │  │  │  │
                                    │  │  │  │  ┌─────────────────┐  │  │  │  │
                                    │  │  │  │  │  envd daemon    │  │  │  │  │
                                    │  │  │  │  │  (process/fs)   │  │  │  │  │
                                    │  │  │  │  └─────────────────┘  │  │  │  │
                                    │  │  │  └───────────────────────┘  │  │  │
                                    │  │  └─────────────────────────────┘  │  │
                                    │  └───────────────────────────────────┘  │
                                    └─────────────────────────────────────────┘
                                                              ▲
                                                              │ FC API
                                                              │
┌──────────────────────────────────────────────────────────────────────────────────┐
│                              E2B-FC Service Layer                                 │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                           API Gateway (Gin)                                 │  │
│  │   - Authentication (API Key, Access Token, Supabase)                       │  │
│  │   - Rate Limiting                                                          │  │
│  │   - Request Validation (OpenAPI)                                           │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
│                                        │                                          │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                          Sandbox Manager                                    │  │
│  │   - Create/List/Get/Kill sandbox                                           │  │
│  │   - Session lifecycle management                                           │  │
│  │   - Template management                                                    │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
│                                        │                                          │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                          FC Session Client                                  │  │
│  │   - FC SDK integration                                                     │  │
│  │   - Session affinity/ isolation mode handling                              │  │
│  │   - Invoke function with session ID                                        │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
│                                        │                                          │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                          Envd Proxy                                         │  │
│  │   - Forward filesystem/process requests to FC session                      │  │
│  │   - WebSocket streaming for terminal/code output                           │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────────┘
                                        ▲
                                        │ HTTP/gRPC
                                        │
                              ┌─────────────────┐
                              │   SDK Clients   │
                              │  (Python/JS/TS) │
                              └─────────────────┘
```

### Component Details

#### 1. API Gateway (packages/api)
Reuse existing API code with modifications:
- Keep: Authentication, OpenAPI validation, rate limiting, logging, telemetry
- Modify: Sandbox handlers to use FC Session instead of orchestrator
- Remove: Firecracker-specific code (NBD, networking, template caching)

#### 2. Sandbox Manager
New component responsible for:
- Managing sandbox lifecycle via FC Session API
- Tracking session IDs and mapping to sandbox IDs
- Handling session TTL and idle timeout
- Coordinating with FC for session affinity

#### 3. FC Session Client
New component wrapping Aliyun FC SDK:
- Create session with isolation mode
- Invoke function with session ID for affinity
- Manage session lifecycle (create, get, delete)
- Handle session state transitions

#### 4. Envd Proxy
Modified version of existing proxy:
- Forward gRPC requests to FC session's envd
- Handle streaming responses
- Manage connection lifecycle

---

## FC Session Integration

### FC Session Concepts

Aliyun FC Session provides two modes:

**Isolation Mode (Recommended)**
- One session per instance
- Complete CPU/memory/disk isolation
- Session ends → instance destroyed
- Ideal for multi-tenant sandbox environments

**Non-Isolation Mode**
- Multiple sessions can reuse an instance
- Lower latency (no cold start after first)
- Suitable for shared environments

### Session Lifecycle Mapping

| E2B Concept | FC Session Concept |
|------------|-------------------|
| Sandbox ID | Session ID |
| Template | Function + Runtime Config |
| Timeout | SessionTTL |
| Idle Timeout | SessionIdleTimeout |
| Sandbox State | Session State |

### Session State Mapping

| E2B State | FC Session State |
|-----------|-----------------|
| Running | Active/Idle |
| Paused | (Not directly supported) |
| Killed | Deleted |

### API Translation

```
E2B API                          FC Session API
─────────────────────────────────────────────────────────────
POST /sandboxes              →   CreateSession + InvokeFunction
GET /sandboxes               →   ListSessions
GET /sandboxes/:id           →   GetSession
DELETE /sandboxes/:id        →   DeleteSession
POST /sandboxes/:id/refresh  →   (Reset SessionIdleTimeout)
```

---

## Sandbox Function Design

### Function Architecture

Each sandbox is backed by an FC function that runs `envd`:

```go
// FC Function Handler
func Handler(ctx context.Context, req events.HTTPRequest) (events.HTTPResponse, error) {
    // 1. Parse request
    // 2. Forward to envd gRPC server
    // 3. Return response
}
```

### Envd in FC Session

The `envd` daemon will be embedded in the FC function:

```
┌─────────────────────────────────────────────────┐
│               FC Function Container              │
│  ┌───────────────────────────────────────────┐  │
│  │              envd daemon                   │  │
│  │  - ProcessService (gRPC)                  │  │
│  │  - FilesystemService (gRPC)               │  │
│  │  - Jupyter Kernel (code execution)        │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │              HTTP Handler                  │  │
│  │  - Route to envd services                 │  │
│  │  - WebSocket for streaming                │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │           Sandbox Environment              │  │
│  │  - /workspace (writable)                  │  │
│  │  - /tmp (temporary)                       │  │
│  │  - Tools (python, node, etc.)             │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

### Function Configuration

```yaml
# FC Function Configuration
Function:
  Runtime: custom-container
  Memory: 2048-8192 MB (configurable)
  Timeout: 600s (max)
  InstanceConcurrency: 1 (isolation mode)

SessionConfig:
  SessionTTL: 3600s (1 hour default)
  SessionIdleTimeout: 1800s (30 min default)
  SessionAffinity: HeaderField (x-session-id)
```

---

## Implementation Plan

### Phase 1: Core Infrastructure

1. **FC Function Setup**
   - Create custom container with envd
   - Configure session isolation mode
   - Set up function invocation routing

2. **Session Manager**
   - Implement FC Session client wrapper
   - Session lifecycle management
   - Session ID to sandbox ID mapping

3. **Basic API Endpoints**
   - POST /sandboxes (create)
   - GET /sandboxes (list)
   - GET /sandboxes/:id (get info)
   - DELETE /sandboxes/:id (kill)

### Phase 2: File System & Commands

4. **File System Service**
   - Implement file operations via envd gRPC
   - Handle file upload/download through FC
   - Directory watching

5. **Terminal Commands**
   - Process execution (sync/async/stream)
   - stdin handling
   - Process lifecycle management

### Phase 3: Code Execution & Browser

6. **Code Execution**
   - Jupyter kernel integration
   - Multi-language support
   - Streaming output

7. **Browser Operations**
   - Browser template support
   - CDP endpoint
   - Live URL generation

### Phase 4: Polish & Testing

8. **SDK Compatibility**
   - Python SDK compatibility
   - JavaScript/TypeScript SDK compatibility

9. **Testing & Documentation**
   - Integration tests
   - API documentation
   - Usage examples

---

## Key Technical Decisions

### 1. Session Affinity Strategy

**Decision**: Use HeaderField affinity with `x-session-id`

**Rationale**:
- Simple to implement
- Compatible with existing e2b SDK patterns
- No MCP dependency

### 2. Template Management

**Decision**: Templates are FC function versions/aliases

**Rationale**:
- FC already supports function versioning
- Alias provides template naming
- Natural fit for template concept

### 3. State Storage

**Decision**: Use Redis for session state cache + FC API for source of truth

**Rationale**:
- Fast lookups for frequent operations
- FC API for authoritative state
- Compatible with existing architecture

### 4. Streaming Strategy

**Decision**: Use WebSocket through FC HTTP trigger

**Rationale**:
- FC supports WebSocket
- Consistent with streaming requirements
- Works with existing envd streaming

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| FC cold start latency | User experience | Use session keepalive, provisioned concurrency |
| Session timeout limits | Long-running tasks | Implement checkpoint/resume pattern |
| No pause/resume native support | API compatibility | Implement soft pause (disconnect, keep session) |
| File persistence | Multi-request files | Use OSS for persistent storage |

---

## Open Questions

1. **Pause/Resume Support**: FC Session doesn't natively support pause. Should we:
   - Implement "soft pause" (disconnect, keep session alive)?
   - Use snapshot mechanism if available?
   - Mark as unsupported?

2. **Template Customization**: How to handle user-defined templates?
   - Build custom FC functions?
   - Use init containers?

3. **Resource Limits**: What are the FC Session resource constraints?
   - Max memory?
   - Max disk?
   - Network access?

---

## Design Review Feedback & Updates

### Technical Decisions (from review)

**1. Streaming Strategy - Updated**
- Use SSE (Server-Sent Events) for stdout/stderr streaming
- Separate HTTP POST endpoint for stdin input
- Simpler than bi-directional WebSocket, works reliably with FC HTTP trigger

**2. Timeout Handling**
- Short operations (< 60s): Sync HTTP invocation
- Long operations: Async pattern with operation ID
- Terminal streaming: SSE + HTTP POST pattern

**3. File Transfer**
- Small files (< 10MB): Direct through FC HTTP
- Large files: OSS presigned URLs
- API responses include `upload_url` / `download_url` for large files

**4. Envd Protocol**
- Keep existing Connect RPC (works over HTTP)
- FC function is HTTP pass-through to envd's Connect endpoints
- No translation layer needed

### Component Decisions

**client-proxy**: Remove. FC handles session affinity routing natively.

**orchestrator**: Abstract with `SandboxProvider` interface:
```go
type SandboxProvider interface {
    Create(ctx context.Context, config SandboxConfig) (Sandbox, error)
    Get(ctx context.Context, id string) (Sandbox, error)
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, opts ListOptions) ([]Sandbox, string, error)
}
```

**Templates**: FC function aliases + metadata registry in Redis/DB.

---

## References

- Aliyun FC Session: https://help.aliyun.com/zh/functioncompute/fc/user-guide/what-is-a-function-session
- Tencent Agent Runtime: https://cloud.tencent.com/document/product/1814/123848
- E2B Infrastructure: /home/rockuw/freeman/e2b-fc