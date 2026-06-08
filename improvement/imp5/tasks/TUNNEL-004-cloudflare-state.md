---
id: TUNNEL-004
domain: tunnel
status: DONE
estimate: 1h
title: Cloudflare State Persistence
---

## Agent Log
- Started: 2026-06-04 17:00
- Completed: 2026-06-04 17:25
- Agent: agent-tunnel
- All AC verified: ✓
- All tests passed: ✓ (covered by TUNNEL-001 tunnel_test.go)

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/tunnel/tunnel.go (StateManager), internal/tunnel/tunnel_test.go
- Implementation detail: StateManager handles all state file operations (SaveState/LoadState/ClearState with atomic writes, graceful missing/corrupt handling, parent dir auto-creation)

## Description

Implement file-based state persistence for Cloudflare tunnel. Manages reading, writing, and clearing the state file that survives process restarts.

## Input

```go
type TunnelState struct {
    ShortID   string `json:"shortId"`
    TunnelURL string `json:"tunnelUrl"`
}
```

## Output

```go
func LoadState(dataDir string) (*TunnelState, error)
func SaveState(dataDir string, state *TunnelState) error
func ClearState(dataDir string) error
```

## Logic

1. State file location: `{DATA_DIR}/tunnel/state.json`
2. Ensure parent directory `{DATA_DIR}/tunnel/` exists before writing
3. SaveState writes atomically (write to temp file, then rename)
4. LoadState returns empty state (not error) if file doesn't exist
5. LoadState returns empty state (not error) if file is corrupt/invalid JSON
6. ClearState removes the state file or writes empty JSON
7. File permissions: readable only by owner (0600)

## Acceptance Criteria
- [x] State file created at `{DATA_DIR}/tunnel/state.json`
- [x] `SaveState` writes valid JSON with correct fields
- [x] `SaveState` uses atomic write (temp + rename)
- [x] `LoadState` reads and parses existing file correctly
- [x] `LoadState` returns empty state for missing file (no error)
- [x] `LoadState` returns empty state for corrupt/invalid JSON
- [x] `ClearState` removes or empties the state file
- [x] Parent directory created automatically if missing
- [x] File permissions set to 0600

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Fresh state | LoadState on empty dir | Empty state, no error |
| Save + load roundtrip | SaveState → LoadState | Identical state |
| Corrupt file | Write "not json" → LoadState | Empty state, no panic |
| Missing fields | JSON with only shortId | Zero-value for tunnelUrl |
| Atomic write | SaveState during crash | No partial/corrupt file |
| Directory missing | SaveState with no parent dir | Directory created, file written |
| Permissions | Check file after SaveState | Mode 0600 |
