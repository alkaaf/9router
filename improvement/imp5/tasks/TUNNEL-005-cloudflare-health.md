---
id: TUNNEL-005
domain: tunnel
status: DONE
estimate: 2h
title: Cloudflare Health Check & DNS Probing
---

## Agent Log
- Started: 2026-06-04 18:26
- Completed: 2026-06-04 18:35
- Agent: agent-tunnel
- All AC verified: ✓
- All tests passed: ✓ (13/13)

## Description

Implement DNS resolution and HTTP health probing for Cloudflare tunnel URLs. Handles DNS propagation delays with retry loops and public DNS resolver fallback.

## Input

```go
type HealthConfig struct {
    DNSTimeout  time.Duration  // Default 2s
    HTTPTimeout time.Duration  // Default 5s
    TotalWait   time.Duration  // Default 60s
    Interval    time.Duration  // Default 2s
    Resolvers   []string       // Default ["1.1.1.1:53", "8.8.8.8:53"]
}
```

## Output

```go
func ProbeURLAlive(ctx context.Context, url string, config HealthConfig) (bool, error)
func WaitForHealth(ctx context.Context, url string, config HealthConfig) error
func ResolveHostname(ctx context.Context, hostname string, resolvers []string) ([]net.IP, error)
```

## Logic

### DNS Resolution
1. Force public DNS resolvers (1.1.1.1, 8.8.8.8) to bypass OS negative cache
2. Try primary resolver first, fall back to secondary
3. Return first successfully resolved IP
4. Timeout per resolver: 2s

### HTTP Health Probe
1. Construct health check URL: `{tunnelUrl}/api/health`
2. Create HTTP client with configured timeout (5s)
3. Send GET request
4. Return true if status == 200

### WaitForHealth Loop
```
elapsed = 0
while elapsed < TotalWait (60s):
    if ctx.Err() != nil: return ctx.Err()
    if ProbeURLAlive(url): return nil
    sleep Interval (2s)
    elapsed += Interval
return timeout error
```

## Acceptance Criteria
- [x] DNS resolution uses public resolvers (1.1.1.1, 8.8.8.8)
- [x] Falls back to secondary resolver on failure
- [x] HTTP probe returns true for 200 response
- [x] HTTP probe returns false for non-200 or timeout
- [x] WaitForHealth retries at configured interval
- [x] WaitForHealth respects context cancellation
- [x] WaitForHealth returns timeout error after TotalWait
- [x] DNS timeout: 2s, HTTP timeout: 5s, Total: 60s, Interval: 2s

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Healthy URL | http://localhost:20128/api/health | true (200 OK) |
| Unreachable URL | http://192.0.2.1:9999/api/health | false (timeout) |
| Non-200 response | Server returns 503 | false |
| DNS resolution | Hostname resolving | IPs returned |
| DNS timeout | Unreachable resolver | Falls back to secondary |
| Context cancel | Cancel during WaitForHealth | Returns ctx.Err() |
| Eventually healthy | URL starts responding at 30s | WaitForHealth returns nil at 30s |
| Permanent failure | URL never responds | Returns timeout error at 60s |

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/tunnel/cloudflare/health.go, internal/tunnel/cloudflare/health_test.go, internal/tunnel/cloudflare/health_adapter.go
