# Tunnel Domain — Migration Breakdown

**Domain:** Tunnel Management (Cloudflare Tunnel + Tailscale)
**Parent Manifest:** `/improvement/imp5/manifest.md`
**Version:** 1.0
**Date:** 2026-06-04

---

## 1. Domain Overview

The Tunnel domain provides network exposure for 9Router via two mechanisms:

| Mechanism | Purpose | Protocol | Auth | Endpoints |
|------------|---------|----------|------|-----------|
| **Cloudflare Tunnel** | Quick public URL via `cloudflared quick-tunnel` | HTTP/2 over QUIC | None (anonymous) | `/api/tunnel/*` |
| **Tailscale Funnel** | Persistent VPN-based exposure via `tailscale funnel` | HTTPS via Tailscale network | Tailscale account required | `/api/tunnel/tailscale-*` |

Both mechanisms expose the local 9Router server (default port 20128) to external clients. The Go backend must reimplement the subprocess management logic currently in `src/lib/tunnel/`.

### 1.1 Key Architectural Constraints

1. **External Process Management**: Both Cloudflare and Tailscale require spawning and managing OS-level child processes. This is inherently platform-specific (macOS/Linux/Windows).

2. **No Native Go SDK Required**: Neither Cloudflare nor Tailscale provides official Go SDKs for quick-tunnel/funnel management. Both use CLI-driven workflows.

3. **State Persistence**: Tunnel state (`shortId`, `tunnelUrl`) must survive restarts. Stored in `{DATA_DIR}/tunnel/state.json`.

4. **DNS Propagation Delays**: Public URLs (Cloudflare `.trycloudflare.com`, Tailscale `*.ts.net`) require DNS propagation time. Health checks must poll with retries.

5. **Cancellation Support**: Long-running tunnel operations (spawn, health check) must support cancellation via context/token.

6. **SSE Streaming for Install**: Tailscale install returns SSE stream for progress updates (not a simple POST/response).

---

## 2. Approach Recommendation

### 2.1 Recommended Approach: Subprocess Management in Go

**Option 2: Spawn subprocess for tunnel management** is the recommended approach because:
- No Go SDKs exist for either Cloudflare quick-tunnel or Tailscale funnel management
- The CLI-driven workflow is well-understood from the Node.js implementation
- Go's `os/exec` and process management are mature and cross-platform
- The Go backend already manages state via GORM, so subprocess management is a natural extension
- CLI stays unchanged (calls Go API endpoints)

### 2.2 Why Not Option 1 (Direct Go SDK Calls)

| Reason | Cloudflare | Tailscale |
|--------|-----------|-----------|
| No official Go SDK | Correct — only `cloudflared` CLI | Correct — only `tailscale` CLI |
| Quick-tunnel API | Not available | N/A |
| Funnel management | N/A | CLI-only interface |

### 2.3 Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                         Go API Server (Fiber)                        │
│                           Port 8090                                   │
├──────────────────────────────────────────────────────────────────────┤
│  /api/tunnel/* Handlers                                              │
│  ├── status      → TunnelService.GetStatus()                         │
│  ├── enable      → TunnelService.Enable()                            │
│  ├── disable     → TunnelService.Disable()                           │
│  ├── tailscale-check    → TailscaleService.CheckStatus()             │
│  ├── tailscale-enable   → TailscaleService.Enable()                  │
│  ├── tailscale-disable  → TailscaleService.Disable()                  │
│  └── tailscale-install  → TailscaleService.Install() [SSE stream]    │
├──────────────────────────────────────────────────────────────────────┤
│  internal/tunnel/                                                    │
│  ├── service.go          # High-level service (cancellation, retry)  │
│  ├── cloudflare/                                                        │
│  │   ├── manager.go      # Cloudflare tunnel lifecycle                │
│  │   ├── cloudflared.go  # Binary download, spawn, kill, health     │
│  │   └── state.go        # State file read/write                     │
│  └── tailscale/                                                        │
│      ├── manager.go      # Tailscale funnel lifecycle                 │
│      ├── tailscale.go    # Binary detection, install, daemon, login  │
│      └── state.go        # State file read/write                     │
├──────────────────────────────────────────────────────────────────────┤
│  os/exec, os/signal, net.Dial, net.Resolver (stdlib)                │
└──────────────────────────────────────────────────────────────────────┘
          │
          │ spawn subprocesses
          ▼
┌─────────────────────┐     ┌─────────────────────┐
│  cloudflared        │     │  tailscale/tailscaled │
│  (Cloudflare Tunnel)│     │  (Tailscale Funnel)   │
│  Port: 7844 (QUIC) │     │  Port: 443 (HTTPS)     │
└─────────────────────┘     └─────────────────────┘
```

### 2.4 Data Flow

1. **Settings** (GORM `settings` table) stores:
   - `tunnelEnabled: bool`
   - `tunnelUrl: string`
   - `tailscaleEnabled: bool`
   - `tailscaleUrl: string`

2. **State File** (`{DATA_DIR}/tunnel/state.json`) stores:
   - `shortId: string` — Generated ID for tunnel identification
   - `tunnelUrl: string` — Direct URL from cloudflared

3. **Binary Storage** (`{DATA_DIR}/bin/`):
   - `cloudflared` — Downloaded from GitHub releases
   - `tailscale`/`tailscaled` — Installed via system package manager or downloaded

---

## 3. Endpoint Contracts

### 3.1 GET /api/tunnel/status

**Purpose:** Get current tunnel state for both Cloudflare and Tailscale.

**Input:** None (query params optional in future)

**Output (200 OK):**
```json
{
  "tunnel": {
    "enabled": true,
    "settingsEnabled": true,
    "tunnelUrl": "https://abc123.trycloudflare.com",
    "shortId": "abc123",
    "publicUrl": "https://rabc123.abc-tunnel.us",
    "running": true
  },
  "tailscale": {
    "enabled": false,
    "settingsEnabled": false,
    "tunnelUrl": "",
    "running": false,
    "loggedIn": false
  },
  "download": {
    "downloading": false,
    "progress": 0
  }
}
```

**Error (500):**
```json
{ "error": "error message" }
```

**Go Handler Signature:**
```go
func GetTunnelStatus(c *fiber.Ctx) error
// Uses: SettingsRepo, TunnelService
// Auth: JWT or CLI token
```

### 3.2 POST /api/tunnel/enable

**Purpose:** Start Cloudflare quick-tunnel.

**Input:** None

**Output (200 OK):**
```json
{
  "success": true,
  "tunnelUrl": "https://abc123.trycloudflare.com",
  "shortId": "abc123",
  "publicUrl": "https://rabc123.abc-tunnel.us",
  "alreadyRunning": false
}
```

**Error (500):**
```json
{ "error": "error message" }
```

**Side Effects:**
- Downloads `cloudflared` binary if not present
- Spawns `cloudflared tunnel --url http://127.0.0.1:20128`
- Writes state to `{DATA_DIR}/tunnel/state.json`
- Updates settings: `tunnelEnabled: true`, `tunnelUrl`
- Registers URL with worker (`https://abc-tunnel.us/api/tunnel/register`)
- Waits for health check (60s timeout)

**Go Handler Signature:**
```go
func EnableTunnel(c *fiber.Ctx) error
// Uses: TunnelService, SettingsRepo, StateManager
// Auth: JWT or CLI token
```

### 3.3 POST /api/tunnel/disable

**Purpose:** Stop Cloudflare tunnel.

**Input:** None

**Output (200 OK):**
```json
{ "success": true }
```

**Error (500):**
```json
{ "error": "error message" }
```

**Side Effects:**
- Kills cloudflared process (direct child + PID file + port-based)
- Clears state: `tunnelUrl: null` (preserves `shortId`)
- Updates settings: `tunnelEnabled: false`

**Go Handler Signature:**
```go
func DisableTunnel(c *fiber.Ctx) error
// Uses: TunnelService, SettingsRepo, StateManager
// Auth: JWT or CLI token
```

### 3.4 GET /api/tunnel/tailscale-check

**Purpose:** Check Tailscale installation and login status.

**Input:** None

**Output (200 OK):**
```json
{
  "installed": true,
  "loggedIn": true,
  "platform": "darwin",
  "brewAvailable": true,
  "daemonRunning": true,
  "hasCachedPassword": true
}
```

**Error (500):**
```json
{ "error": "error message" }
```

**Notes:**
- `hasCachedPassword` is always `true` in Go (password managed externally)
- Daemon running check uses `tailscale status --json` via socket

**Go Handler Signature:**
```go
func TailscaleCheck(c *fiber.Ctx) error
// Uses: TailscaleService
// Auth: JWT or CLI token
```

### 3.5 POST /api/tunnel/tailscale-install

**Purpose:** Install Tailscale binary with SSE progress stream.

**Input (JSON body, optional):**
```json
{
  "sudoPassword": "secret"
}
```

**Output (SSE stream):**
```
event: progress
data: {"message": "Downloading Tailscale package..."}

event: progress
data: {"message": "Installing package..."}

event: done
data: {"success": true, "authUrl": null}

--- OR (if login required) ---

event: done
data: {"success": true, "authUrl": "https://login.tailscale.com/a/abc123..."}
```

**Error Events:**
```
event: error
data: {"error": "Wrong sudo password"}
```

**Notes:**
- Requires `sudoPassword` on Linux/macOS (non-brew), optional on Windows
- Auth URL returned if Tailscale login required post-install
- Windows uses MSI installer with UAC elevation

**Go Handler Signature:**
```go
func TailscaleInstall(c *fiber.Ctx) error
// Uses: TailscaleService
// Returns: Fiber SSE stream (text/event-stream)
// Auth: JWT or CLI token
```

### 3.6 POST /api/tunnel/tailscale-enable

**Purpose:** Start Tailscale daemon and enable funnel.

**Input:** None

**Output (200 OK - success):**
```json
{
  "success": true,
  "tunnelUrl": "https://device-name.tail123.ts.net"
}
```

**Output (200 OK - needs login):**
```json
{
  "success": false,
  "needsLogin": true,
  "authUrl": "https://login.tailscale.com/a/abc123..."
}
```

**Output (200 OK - funnel not enabled):**
```json
{
  "success": false,
  "funnelNotEnabled": true,
  "enableUrl": "https://login.tailscale.com/..."
}
```

**Error (500):**
```json
{ "error": "error message" }
```

**Side Effects:**
- Starts `tailscaled` daemon (TUN mode with sudo, or userspace-networking without)
- Runs `tailscale up --accept-routes --hostname=<shortId>`
- Runs `tailscale funnel --bg 20128`
- Provisions TLS cert for funnel domain
- Health checks funnel URL (180s timeout, non-fatal on timeout)

**Go Handler Signature:**
```go
func TailscaleEnable(c *fiber.Ctx) error
// Uses: TailscaleService, SettingsRepo
// Auth: JWT or CLI token
```

### 3.7 POST /api/tunnel/tailscale-disable

**Purpose:** Stop Tailscale funnel.

**Input:** None

**Output (200 OK):**
```json
{ "success": true }
```

**Error (500):**
```json
{ "error": "error message" }
```

**Side Effects:**
- Runs `tailscale funnel --bg reset`
- Updates settings: `tailscaleEnabled: false`

**Go Handler Signature:**
```go
func TailscaleDisable(c *fiber.Ctx) error
// Uses: TailscaleService, SettingsRepo
// Auth: JWT or CLI token
```

---

## 4. File Structure

```
internal/tunnel/
├── tunnel.go                    # Public API: TunnelService, TailscaleService
├── tunnel_test.go
├── cloudflare/
│   ├── manager.go               # enableTunnel, disableTunnel, getTunnelStatus
│   ├── cloudflared.go           # Binary download, spawn, kill, health check
│   ├── state.go                 # State file (shortId, tunnelUrl)
│   ├── health.go                # DNS resolve + HTTP probe + waitForHealth
│   └── cloudflare_test.go
└── tailscale/
    ├── manager.go               # enableTailscale, disableTailscale, getTailscaleStatus
    ├── tailscale.go             # Binary detection, install, daemon, login, funnel
    ├── state.go                 # State file (shortId)
    ├── health.go                # DNS resolve + HTTP probe + waitForHealth
    └── tailscale_test.go
```

---

## 5. Atomic Tasks

### TUNNEL-001: Tunnel Service Infrastructure

**Description:** Create the tunnel service package with type definitions, state management, and cancellation support.

**Files Created:**
- `internal/tunnel/tunnel.go` — Service types, interfaces, and public API

**Input:**
```go
type TunnelConfig struct {
    DataDir      string  // Base data directory (e.g., ~/.9router)
    LocalPort    int     // Local server port (default 20128)
    WorkerURL    string  // Cloudflare worker URL for registration
}

type TunnelState struct {
    ShortID   string `json:"shortId"`
    TunnelURL string `json:"tunnelUrl"`
}
```

**Output:**
- `TunnelService` struct with methods: `Enable()`, `Disable()`, `GetStatus()`
- `TailscaleService` struct with methods: `Check()`, `Install()`, `Enable()`, `Disable()`, `GetStatus()`
- `StateManager` for reading/writing `{DATA_DIR}/tunnel/state.json`
- Context cancellation support for long-running operations

**Test Strategy:**
- Unit test state file read/write roundtrip
- Unit test short ID generation (6-char alphanumeric)
- Mock filesystem operations for state file

**Dependencies:** None (core package)

---

### TUNNEL-002: Cloudflare Binary Management

**Description:** Implement cloudflared binary download, validation, and process management.

**Files Created:**
- `internal/tunnel/cloudflare/cloudflared.go`
- `internal/tunnel/cloudflare/cloudflared_test.go`

**Input:**
- Platform (darwin/linux/win32)
- Architecture (amd64/arm64)

**Output:**
- `EnsureCloudflared(ctx context.Context, progressFn func(int)) (string, error)` — Downloads and validates binary
- `SpawnQuickTunnel(ctx context.Context, port int, onUrlUpdate func(string)) (child *exec.Cmd, tunnelUrl string, err error)`
- `KillCloudflared(port int) error`
- `IsCloudflaredRunning() bool`

**Binary Download URL Pattern:**
```
https://github.com/cloudflare/cloudflared/releases/latest/download/
  ├── cloudflared-darwin-amd64.tgz
  ├── cloudflared-darwin-arm64.tgz
  ├── cloudflared-linux-amd64
  ├── cloudflared-linux-arm64
  ├── cloudflared-windows-amd64.exe
  └── cloudflared-windows-386.exe
```

**Validation:**
- Minimum file size check (1MB)
- Magic bytes: ELF (`7f454c46`), PE/MZ (`4d5a`), Mach-O (`cffaedfe`/`cefaedfe`)

**Test Strategy:**
- Unit test download URL construction per platform
- Unit test binary validation (mock file with magic bytes)
- Integration test: download and spawn (requires network)

**Dependencies:** TUNNEL-001

---

### TUNNEL-003: Cloudflare Tunnel Manager

**Description:** Implement high-level Cloudflare tunnel lifecycle management.

**Files Created:**
- `internal/tunnel/cloudflare/manager.go`
- `internal/tunnel/cloudflare/manager_test.go`

**Input:**
- Context with cancellation support
- Local port (default 20128)

**Output:**
- `Enable(ctx context.Context) (TunnelResult, error)`
- `Disable(ctx context.Context) error`
- `GetStatus(ctx context.Context) (TunnelStatus, error)`

**Key Logic:**
1. Check if already running with health probe (both direct + public URL)
2. Kill existing cloudflared if stale
3. Spawn quick tunnel with config file in temp directory
4. Register URL with worker
5. Save state (shortId, tunnelUrl)
6. Update settings (tunnelEnabled, tunnelUrl)
7. Wait for health (60s timeout)
8. Return result

**Health Check Loop:**
```go
for elapsed < 60000ms {
    if ctx.Err() != nil { return error }
    if probeUrl(url) { return success }
    sleep(2000ms)
}
return timeout error
```

**Test Strategy:**
- Unit test state file interaction
- Unit test URL parsing and construction
- Mock subprocess spawn for integration test

**Dependencies:** TUNNEL-002

---

### TUNNEL-004: Cloudflare Health Check

**Description:** Implement DNS resolution and HTTP health probe for Cloudflare URLs.

**Files Created:**
- `internal/tunnel/cloudflare/health.go`
- `internal/tunnel/cloudflare/health_test.go`

**Input:**
- URL to probe
- Timeout configuration

**Output:**
- `ProbeUrlAlive(ctx context.Context, url string) (bool, error)`
- `WaitForHealth(ctx context.Context, url string, timeout time.Duration) error`

**DNS Resolution:**
```go
// Force public DNS (1.1.1.1, 8.8.8.8) to bypass OS negative cache
resolver := net.LookupIP
// Try resolver 1 first, fall back to resolver 2
```

**Health Check:**
```go
resp, err := httpClient.Get(url + "/api/health")
return resp.StatusCode == 200
```

**Timeouts:**
| Phase | Timeout |
|-------|---------|
| DNS resolution | 2s |
| HTTP probe | 5s |
| Total wait | 60s |
| Interval | 2s |

**Test Strategy:**
- Unit test DNS resolution with mock resolver
- Unit test health check with mock HTTP server

**Dependencies:** TUNNEL-001

---

### TUNNEL-005: Cloudflare State Management

**Description:** Implement state file read/write for Cloudflare tunnel persistence.

**Files Created:**
- `internal/tunnel/cloudflare/state.go`
- `internal/tunnel/cloudflare/state_test.go`

**Input:** None (file-based)

**Output:**
- `LoadState() (*TunnelState, error)`
- `SaveState(state *TunnelState) error`
- `ClearState() error`

**File Location:**
```
{DATA_DIR}/tunnel/state.json
```

**File Format:**
```json
{
  "shortId": "abc123",
  "tunnelUrl": "https://abc123.trycloudflare.com"
}
```

**Test Strategy:**
- Unit test roundtrip read/write with temp file
- Unit test graceful handling of missing/corrupt file

**Dependencies:** TUNNEL-001

---

### TUNNEL-006: Tailscale Binary Management

**Description:** Implement Tailscale binary detection and installation.

**Files Created:**
- `internal/tunnel/tailscale/tailscale.go`
- `internal/tunnel/tailscale/tailscale_test.go`

**Input:**
- Platform (darwin/linux/win32)
- Installation method (brew/homebrew-pkg/linux-script/windows-msi)

**Output:**
- `IsTailscaleInstalled() bool`
- `GetTailscaleBin() string`
- `Install(ctx context.Context, sudoPassword string, shortId string, progressFn func(string)) (InstallResult, error)`

**Installation Methods:**

| Platform | Method | Command |
|----------|--------|---------|
| macOS + brew | `brew install tailscale` | Non-interactive |
| macOS no brew | Download `.pkg` | `sudo installer -pkg tailscale.pkg -target /` |
| Linux | `curl -fsSL tailscale.com/install.sh \| sh` | Sudo via stdin |
| Windows | Download `.msi` | `msiexec /i tailscale-setup.msi` via UAC |

**Sudo Password Handling:**
```go
// Pipe password to sudo stdin
cmd := exec.Command("sudo", "-S", "installer", ...)
cmd.Stdin = strings.NewReader(sudoPassword + "\n")
```

**Test Strategy:**
- Unit test binary detection paths
- Unit test install command construction
- Integration test: install (requires network, sudo)

**Dependencies:** TUNNEL-001

---

### TUNNEL-007: Tailscale Daemon Management

**Description:** Implement tailscaled daemon lifecycle (start, stop, status).

**Files Created:**
- `internal/tunnel/tailscale/daemon.go` (merged into tailscale.go)
- `internal/tunnel/tailscale/daemon_test.go`

**Input:**
- Sudo password (optional, enables TUN mode)
- Custom socket path

**Output:**
- `StartDaemon(ctx context.Context, sudoPassword string) error`
- `StopDaemon(ctx context.Context, sudoPassword string) error`
- `IsDaemonRunning() bool`
- `IsLoggedIn() bool`

**Socket Path:**
```
{DATA_DIR}/tailscale/tailscaled.sock
```

**Daemon Arguments:**
```go
args := []string{
    "--socket=" + socketPath,
    "--statedir=" + stateDir,
}
// Without sudo: add "--tun=userspace-networking"
```

**Test Strategy:**
- Unit test daemon argument construction
- Mock subprocess spawn for integration test

**Dependencies:** TUNNEL-006

---

### TUNNEL-008: Tailscale Login Flow

**Description:** Implement Tailscale login with auth URL capture.

**Files Created:**
- `internal/tunnel/tailscale/login.go` (merged into tailscale.go)
- `internal/tunnel/tailscale/login_test.go`

**Input:**
- Hostname (shortId)
- Daemon socket path

**Output:**
- `Login(ctx context.Context, hostname string) (LoginResult, error)`

**Result Types:**
```go
type LoginResult struct {
    AuthURL         string  // URL for browser login (if needsLogin)
    AlreadyLoggedIn bool
}

type LoginStatus int
const (
    LoginStatusNeedsAuth LoginStatus = iota
    LoginStatusLoggedIn
    LoginStatusTimeout
)
```

**Login Flow:**
1. Run `tailscale up --accept-routes --hostname=<shortId>`
2. Parse stdout/stderr for `https://login.tailscale.com/a/...`
3. Poll `tailscale status --json` for BackendState
4. Poll for AuthURL in status JSON (Windows-specific)
5. Timeout after 15s

**Test Strategy:**
- Unit test URL parsing from output
- Mock subprocess for integration test

**Dependencies:** TUNNEL-007

---

### TUNNEL-009: Tailscale Funnel Management

**Description:** Implement Tailscale funnel (port exposure) lifecycle.

**Files Created:**
- `internal/tunnel/tailscale/funnel.go` (merged into tailscale.go)
- `internal/tunnel/tailscale/funnel_test.go`

**Input:**
- Local port (20128)
- Daemon socket path

**Output:**
- `StartFunnel(ctx context.Context, port int) (FunnelResult, error)`
- `StopFunnel() error`
- `GetFunnelStatus() bool`
- `ProvisionCert(hostname string) error`

**Funnel Flow:**
1. Reset existing funnel: `tailscale funnel --bg reset`
2. Enable funnel: `tailscale funnel --bg <port>`
3. Parse output for `Funnel is not enabled` (needs admin console)
4. Extract hostname from `tailscale status --json` → `Self.DNSName`
5. Provision TLS cert: `tailscale cert --cert-file <crt> --key-file <key> <hostname>`
6. Health check funnel URL (180s timeout)

**Result Types:**
```go
type FunnelResult struct {
    TunnelUrl        string  // https://<hostname>.ts.net
    FunnelNotEnabled bool
    EnableUrl        string  // URL to enable in Tailscale admin console
}
```

**Test Strategy:**
- Unit test command construction
- Mock subprocess for integration test

**Dependencies:** TUNNEL-007, TUNNEL-008

---

### TUNNEL-010: Tailscale Manager

**Description:** Implement high-level Tailscale service orchestrating daemon, login, and funnel.

**Files Created:**
- `internal/tunnel/tailscale/manager.go`
- `internal/tunnel/tailscale/manager_test.go`

**Input:**
- Context with cancellation
- Local port (20128)

**Output:**
- `Check(ctx context.Context) (TailscaleCheckResult, error)`
- `Install(ctx context.Context, sudoPassword string) (SSEStream, error)`
- `Enable(ctx context.Context) (TailscaleEnableResult, error)`
- `Disable(ctx context.Context) error`
- `GetStatus(ctx context.Context) (TailscaleStatus, error)`

**Enable Orchestration:**
1. Start daemon with password
2. Check login status
3. If not logged in, return `needsLogin: true, authUrl: <url>`
4. Start funnel
5. If funnel not enabled, return `funnelNotEnabled: true, enableUrl: <url>`
6. Provision TLS cert
7. Wait for health (non-fatal on timeout)
8. Update settings

**Test Strategy:**
- Unit test with mocked dependencies
- Integration test with real Tailscale (requires account)

**Dependencies:** TUNNEL-006, TUNNEL-007, TUNNEL-008, TUNNEL-009

---

### TUNNEL-011: Tunnel Handlers

**Description:** Implement Fiber HTTP handlers for all tunnel endpoints.

**Files Created:**
- `internal/handler/api/tunnel.go`
- `internal/handler/api/tunnel_test.go`

**Endpoints:**
| Method | Path | Handler | Stream |
|--------|------|---------|--------|
| GET | `/api/tunnel/status` | `GetTunnelStatus` | No |
| POST | `/api/tunnel/enable` | `EnableTunnel` | No |
| POST | `/api/tunnel/disable` | `DisableTunnel` | No |
| GET | `/api/tunnel/tailscale-check` | `TailscaleCheck` | No |
| POST | `/api/tunnel/tailscale-install` | `TailscaleInstall` | Yes (SSE) |
| POST | `/api/tunnel/tailscale-enable` | `TailscaleEnable` | No |
| POST | `/api/tunnel/tailscale-disable` | `TailscaleDisable` | No |

**SSE Streaming for TailscaleInstall:**
```go
func TailscaleInstall(c *fiber.Ctx) error {
    c.Set("Content-Type", "text/event-stream")
    c.Set("Cache-Control", "no-cache")
    c.Set("Connection", "keep-alive")

    // Stream progress events
    progressFn := func(msg string) {
        c.Context().Write([]byte(fmt.Sprintf("event: progress\ndata: {\"message\": %q}\n\n", msg)))
    }

    result, err := ts.Install(ctx, sudoPassword, shortId, progressFn)
    // Send done or error event
}
```

**Auth:** All endpoints require JWT or CLI token (`x-9r-cli-token`)

**Test Strategy:**
- Unit test request parsing
- Integration test with httptest

**Dependencies:** TUNNEL-003, TUNNEL-010

---

### TUNNEL-012: Tunnel Router Registration

**Description:** Implement Cloudflare worker registration endpoint.

**Files Created:**
- `internal/tunnel/cloudflare/worker.go` (minimal client)
- `internal/tunnel/cloudflare/worker_test.go`

**Input:**
- Worker URL (e.g., `https://abc-tunnel.us`)
- Short ID
- Tunnel URL

**Output:**
- `RegisterTunnel(ctx context.Context, workerURL, shortId, tunnelUrl string) error`

**Request:**
```http
POST /api/tunnel/register HTTP/1.1
Host: abc-tunnel.us
Content-Type: application/json

{"shortId": "abc123", "tunnelUrl": "https://abc123.trycloudflare.com"}
```

**Test Strategy:**
- Unit test request construction
- Mock HTTP server for integration test

**Dependencies:** TUNNEL-001

---

## 6. Task Dependency Graph

```
TUNNEL-001 (Service Infrastructure)
    │
    ├──────────────────────────────┬──────────────────────────────┐
    │                              │                              │
    ▼                              ▼                              ▼
TUNNEL-002                     TUNNEL-005                     TUNNEL-006
(Cloudflare Binary)             (CF State)                    (Tailscale Binary)
    │                              │                              │
    ▼                              ▼                              │
TUNNEL-004                     TUNNEL-003                     TUNNEL-007
(CF Health Check)               (CF Manager)                   (TS Daemon)
    │                                                         │
    │                                                         ▼
    │                                                    TUNNEL-008
    │                                                    (TS Login)
    │                                                         │
    │                                                         ▼
    │                                                    TUNNEL-009
    │                                                    (TS Funnel)
    │                                                         │
    │                                                         ▼
    │                                                    TUNNEL-010
    │                                                    (TS Manager)
    │                                                         │
    ├─────────────────────────────────────────────────────────┤
    │                                                         │
    ▼                                                         ▼
TUNNEL-012                                                TUNNEL-011
(CF Worker Registration)                                   (Handlers)
```

---

## 7. Test Strategy Summary

| Task | Unit Tests | Integration Tests | E2E Tests |
|------|------------|-------------------|-----------|
| TUNNEL-001 | State read/write, short ID gen | - | - |
| TUNNEL-002 | URL construction, binary validation | Download + spawn | - |
| TUNNEL-003 | State interaction, URL parsing | Spawn tunnel | - |
| TUNNEL-004 | DNS mock, HTTP mock | Health check loop | - |
| TUNNEL-005 | File roundtrip | - | - |
| TUNNEL-006 | Detection paths, command build | Install | - |
| TUNNEL-007 | Argument construction | Start/stop daemon | - |
| TUNNEL-008 | URL parsing | Login flow | - |
| TUNNEL-009 | Command construction | Funnel enable | - |
| TUNNEL-010 | Mocked orchestration | - | Real TS account |
| TUNNEL-011 | Request parsing | httptest handlers | - |
| TUNNEL-012 | Request construction | Mock worker | - |

**Total Estimated Test Cases:** 50+

---

## 8. Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TUNNEL_WORKER_URL` | `https://abc-tunnel.us` | Cloudflare worker for tunnel registration |
| `TUNNEL_LOCAL_PORT` | `20128` | Local server port to expose |
| `TUNNEL_TRANSPORT_PROTOCOL` | `http2` | Cloudflare transport (http2/quic/auto) |
| `DATA_DIR` | `~/.9router` | Base data directory |

### Settings Table Fields (GORM)

The `settings` table stores tunnel configuration:

```go
type Settings struct {
    ID   uint   `gorm:"primaryKey;check:id = 1"`
    Data string `gorm:"not null;type:text"` // JSON
}

// JSON fields relevant to tunnel:
type TunnelSettings struct {
    TunnelEnabled    bool   `json:"tunnelEnabled"`
    TunnelUrl        string `json:"tunnelUrl"`
    TailscaleEnabled bool   `json:"tailscaleEnabled"`
    TailscaleUrl     string `json:"tailscaleUrl"`
}
```

---

## 9. Error Handling

### Error Categories

| Category | Example | HTTP Status | Recovery |
|----------|---------|-------------|----------|
| Binary download | Network timeout | 500 | Retry with exponential backoff |
| Binary validation | Corrupt file | 500 | Delete and re-download |
| Spawn timeout | Tunnel didn't start | 500 | Kill and retry |
| Health check timeout | DNS not propagated | 200 (warning) | Log, continue |
| Auth required | Tailscale login needed | 200 (needsLogin) | Return auth URL |
| Funnel not enabled | TS admin not configured | 200 (funnelNotEnabled) | Return enable URL |
| Wrong password | Sudo password incorrect | 400 | Return error |
| Process killed | User cancelled | 200 | Clean state |

### Cancellation Handling

All long-running operations must check context cancellation:

```go
func enableTunnel(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            // Continue operation
        }

        // Check token-based cancellation too
        if cancelToken.cancelled {
            return errors.New("tunnel cancelled")
        }
    }
}
```

---

## 10. Security Considerations

### Sudo Password Handling

1. **Never log passwords** — redact in all log output
2. **Stdin piping** — pass password via `stdin`, not command line
3. **Injection prevention** — reject passwords with newlines
4. **Temporary files** — for Tailscale install script, write to temp file with restricted permissions

### Binary Validation

1. **Size check** — minimum 1MB for cloudflared
2. **Magic bytes** — verify executable format per platform
3. **Permissions** — chmod 755 after download

### Process Isolation

1. **Detached processes** — use `SysProcAttr.Setpgid = true`
2. **Cleanup on exit** — ensure processes are killed on server shutdown
3. **Resource limits** — set process group kill for cleanup

---

## 11. Platform-Specific Notes

### macOS

- **Homebrew path:** `/opt/homebrew/bin` (Apple Silicon), `/usr/local/bin` (Intel)
- **System paths:** `/usr/local/bin`, `/usr/bin`
- **Tailscale install:** Prefer `brew install tailscale` (no sudo)
- **Cloudflared:** Download `.tgz`, extract to `{DATA_DIR}/bin/`

### Linux

- **System paths:** `/usr/local/bin`, `/usr/bin`
- **Tailscale install:** Use install script with sudo
- **Cloudflared:** Download binary directly (no archive)

### Windows

- **Tailscale:** MSI installer via UAC elevation
- **Cloudflared:** `.exe` download
- **Socket:** N/A (use Windows service API)
- **Process kill:** PowerShell `Stop-Process`

---

## 12. Migration Checklist

- [ ] TUNNEL-001: Service infrastructure
- [ ] TUNNEL-002: Cloudflare binary management
- [ ] TUNNEL-003: Cloudflare tunnel manager
- [ ] TUNNEL-004: Cloudflare health check
- [ ] TUNNEL-005: Cloudflare state management
- [ ] TUNNEL-006: Tailscale binary management
- [ ] TUNNEL-007: Tailscale daemon management
- [ ] TUNNEL-008: Tailscale login flow
- [ ] TUNNEL-009: Tailscale funnel management
- [ ] TUNNEL-010: Tailscale manager
- [ ] TUNNEL-011: Tunnel handlers
- [ ] TUNNEL-012: Worker registration

---

## 13. Open Questions

1. **Worker registration persistence:** The Cloudflare worker at `https://abc-tunnel.us` registers tunnel URLs. Who manages this worker? Is it part of 9Router infrastructure?

2. **Tailscale auth callback:** After user visits auth URL, how does the Go server know login completed? Poll `tailscale status`?

3. **Windows socket:** How to handle Tailscale socket on Windows? Skip custom socket?

4. **Funnel HTTPS:** Tailscale funnel with TUN mode serves HTTPS. With userspace-networking, HTTPS requires cert provisioning. Which mode to default to?

---

*Document Version: 1.0*
*Last Updated: 2026-06-04*
*Author: Claude Code*
