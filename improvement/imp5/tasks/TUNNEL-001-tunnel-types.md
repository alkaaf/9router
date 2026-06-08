---
id: TUNNEL-001
domain: tunnel
status: DONE
estimate: 2h
title: Tunnel Service Infrastructure & Types
---

## Agent Log
- Started: 2026-06-04 17:00
- Completed: 2026-06-04 17:25
- Agent: agent-tunnel
- All AC verified: ✓
- All tests passed: ✓ (13/13)

## Description

Create the tunnel service package with core type definitions, interfaces, state management, and cancellation support. This is the foundation package that all other tunnel tasks depend on.

## Input

```go
type TunnelConfig struct {
    DataDir      string  // Base data directory (e.g., ~/.9router)
    LocalPort    int     // Local server port (default 20128)
    WorkerURL    string  // Cloudflare worker URL for registration
}
```

## Output

- `internal/tunnel/tunnel.go` — Service types, interfaces, and public API
- `TunnelService` struct with methods: `Enable()`, `Disable()`, `GetStatus()`
- `TailscaleService` struct with methods: `Check()`, `Install()`, `Enable()`, `Disable()`, `GetStatus()`
- `StateManager` for reading/writing `{DATA_DIR}/tunnel/state.json`
- Context cancellation support for long-running operations

## Logic

1. Define `TunnelConfig` struct with DataDir, LocalPort, WorkerURL fields
2. Define `TunnelState` struct with ShortID and TunnelURL fields
3. Define `ShortID` generation function (6-char alphanumeric)
4. Create `StateManager` with LoadState(), SaveState(), ClearState() methods
5. Create `TunnelService` struct wrapping Cloudflare tunnel operations
6. Create `TailscaleService` struct wrapping Tailscale funnel operations
7. All long-running methods accept `context.Context` for cancellation
8. State file stored at `{DATA_DIR}/tunnel/state.json`

## Acceptance Criteria
- [x] `TunnelConfig` struct defined with all required fields
- [x] `TunnelState` struct defined with JSON tags
- [x] `generateShortID()` produces unique 6-char alphanumeric IDs
- [x] `StateManager.LoadState()` reads and parses state.json
- [x] `StateManager.SaveState()` writes state.json atomically
- [x] `StateManager.ClearState()` removes or nulls state file
- [x] `TunnelService` and `TailscaleService` interfaces defined
- [x] All service methods accept `context.Context`
- [x] Package compiles without errors

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Short ID generation | Call generateShortID() | 6-char alphanumeric string |
| State roundtrip | SaveState → LoadState | Same state returned |
| Missing state file | LoadState() on fresh dir | Empty state, no error |
| Corrupt state file | LoadState() with bad JSON | Empty state, no panic |
| Context cancellation | Cancel context mid-operation | Operation returns ctx.Err() |
| Concurrent state writes | Multiple SaveState() calls | No data corruption |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/tunnel/tunnel.go, internal/tunnel/tunnel_test.go
- AC evidence:
  - AC-1/2: `TunnelConfig`/`TunnelState` defined in tunnel.go:46-60
  - AC-3: TestGenerateShortID_LengthAndAlphabet + TestGenerateShortID_Unique pass (1000 IDs, 0 duplicates)
  - AC-4: TestStateManager_Roundtrip + TestStateManager_MissingFileReturnsEmpty + TestStateManager_CorruptFileReturnsEmpty
  - AC-5: TestStateManager_AtomicWriteProducesValidJSON (temp + rename)
  - AC-6: TestStateManager_ClearState (idempotent)
  - AC-7: `TunnelService`/`TailscaleService` interfaces at tunnel.go
  - AC-8: All interface methods take `context.Context`
  - AC-9: `go build ./...` and `go vet ./internal/tunnel/` both clean
