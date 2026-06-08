# 9Router Backend Rewrite — Agent Workflow

**Version:** 1.0
**Date:** 2026-06-04
**Purpose:** Workflow untuk agent executor team mengerjakan 130 atomic tasks

---

## 1. Core Principles

1. **1 task = 1 file** — Setiap task ada di `tasks/{TASK-ID}-{name}.md`
2. **Atomic & independent** — Tidak ada dependency lintas domain dalam 1 file
3. **Status wajib di-update** — Agent harus update status di setiap milestone
4. **Test wajib lulus** — Semua acceptance criteria harus terpenuhi
5. **Loop until success** — Jika test gagal, fix dan retry, bukan pindah task

---

## 2. Task File Structure

Setiap task file mengikuti format ini:

```markdown
---
id: AUTH-001
domain: auth
status: TODO          # Agent yang mengubah ini
estimate: 1h
title: JWT Struct Definition
---

## Description
[One specific thing this task does]

## Input
[Exact Go input types and fields]

## Output
[Exact Go output types and fields]

## Logic
[Step-by-step logic, 5-15 lines max, numbered]

## Acceptance Criteria
- [ ] Criterion 1        # Agent yang centang ini
- [ ] Criterion 2

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| [name] | [input] | [output] |
```

---

## 3. Status Values

| Status | Arti | Kapan |
|--------|------|-------|
| `TODO` | Belum dikerjakan | Initial state |
| `IN_PROGRESS` | Sedang dikerjakan | Agent mulai implement |
| `DONE` | Selesai & test lulus | Semua acceptance criteria terpenuhi |
| `BLOCKED` | Terhalang dependency | Task lain yang belum done |
| `NEEDS_REVIEW` | Menunggu review manusia | Opsional, untuk high-risk task |

---

## 4. Agent Workflow Loop

```
LOOP until semua task bernilai DONE:

  1. SCAN tasks/ untuk status=TODO atau IN_PROGRESS
     
  2. FILTER by domain (agent hanya kerja di domain-nya)
     
  3. CHECK dependencies:
     - Baca semua field dependency di task file
     - Cek setiap dependency: statusnya DONE?
     - Jika ada yang belum DONE → skip task ini, lanjut ke berikutnya
     
  4. CLAIM task:
     - Ganti frontmatter: status: TODO → status: IN_PROGRESS
     - Tulis timestamp di bagian bawah file:
       ## Agent Log
       - Started: YYYY-MM-DD HH:MM
       - Agent: {agent-name}
     
  5. IMPLEMENT code:
     - Baca Input, Output, Logic dari task file
     - Tulis code sesuai Logic (step-by-step)
     - Simpan di lokasi yang benar (lihat section 6)
     
  6. RUN test scenarios:
     - Untuk setiap row di Test Scenarios table:
       a. Siapkan input sesuai kolom Input
       b. Jalankan function yang diimplementasikan
       c. Bandingkan output dengan Expected Output
       d. Jika match: lanjut ke scenario berikutnya
       e. Jika mismatch: BLOKIR → status=BLOCKED, log error, hentikan task
     
  7. VERIFY acceptance criteria:
     - Untuk setiap checkbox di Acceptance Criteria:
       a. Ganti `- [ ]` menjadi `- [x]`
       b. Tulis timestamp dan bukti di Agent Log:
          ## Agent Log
          - Started: YYYY-MM-DD HH:MM
          - Completed: YYYY-MM-DD HH:MM
          - Agent: {agent-name}
          - AC-001 verified: [brief evidence]
          - AC-002 verified: [brief evidence]
     
  8. FINALIZE:
     - Ganti frontmatter: status: IN_PROGRESS → status: DONE
     - Tulis ringkasan di akhir file:
       ## Completion
       - All acceptance criteria: ✓
       - All test scenarios: ✓
       - Code location: {path}
       - Commit hash: (jika ada git)
```

---

## 5. Agent Responsibilities

### 5.1 Mandatory Actions

Agent WAJIB melakukan hal-hal berikut:

```
□ Update status: TODO → IN_PROGRESS saat mulai
□ Update status: IN_PROGRESS → DONE saat selesai
□ Centang SEMUA acceptance criteria (- [ ] → - [x])
□ Jalankan SEMUA test scenarios
□ Jika ada yang gagal: status → BLOCKED, jangan lanjut
□ Tulis Agent Log dengan timestamp
```

### 5.2 Prohibited Actions

Agent DILARANG:

```
□ Melewati task yang dependencies-nya belum DONE
□ Menandai DONE tanpa semua AC terpenuhi
□ Menandai DONE tanpa semua test lulus
□ Mengubah konten task file selain frontmatter status dan checkbox AC
□ Membuat code di lokasi yang salah
```

### 5.3 Blocked Task Handling

Jika task stuck di `BLOCKED`:

```
1. Di file task, tambahkan:
   ## Blocked Reason
   - Blocker: {dependency-task-id} belum DONE
   - Blocker: {alasan lain}
   - Since: YYYY-MM-DD

2. Laporkan ke master agent

3. Jangan coba "workaround" — tunggu dependency resolved
```

---

## 6. Code Location Map

Agent harus menulis code di lokasi yang benar:

| Domain | Source Location | Test Location |
|--------|----------------|---------------|
| AUTH | `internal/auth/`, `internal/middleware/auth.go` | `internal/auth/*_test.go` |
| DB | `internal/model/`, `internal/repository/` | `internal/repository/*_test.go` |
| PROV | `internal/repository/provider.go`, `internal/handler/api/providers.go` | `internal/handler/api/providers_test.go` |
| COMBO | `internal/combo/` | `internal/combo/*_test.go` |
| CHAT | `internal/handler/v1/chat.go`, `internal/combo/` | `internal/handler/v1/chat_test.go` |
| EXEC | `internal/executor/` | `internal/executor/*_test.go` |
| USAGE | `internal/repository/usage.go`, `internal/handler/api/usage.go` | `internal/handler/api/usage_test.go` |
| SYS | `internal/handler/api/` | `internal/handler/api/*_test.go` |
| TUNNEL | `internal/tunnel/` | `internal/tunnel/*_test.go` |
| OAUTH | `internal/handler/api/oauth.go`, etc | `internal/handler/api/oauth_test.go` |

---

## 7. Dependency Rules

### 7.1 Hard Dependencies (MUST wait)

```
DB-001 → DB-002 → ... → DB-015  (sequential, each builds on previous)
DB-001 → AUTH-001 (JWT uses no DB, but AUTH handlers need DB)
DB-001 → PROV-001 (model resolution needs DB)
DB-001 → CHAT-001 (chat handler needs DB)
DB-001 → EXEC-001 (executor needs config from DB)
DB-004 → SYS-001 (settings uses DB)
```

### 7.2 Soft Dependencies (can parallel)

```
AUTH-001 → AUTH-002 → ...  (sequential within domain)
PROV-001 → PROV-002 → ...  (sequential within domain)
CHAT-001 → CHAT-002 → ...  (sequential within domain)
TUNNEL-*  (fully independent, can start immediately)
OAUTH-*   (fully independent, can start immediately)
```

### 7.3 Dependency Check Algorithm

```
function canExecute(task):
    for each dep in task.dependencies:
        depFile = findTaskFile(dep)
        if depFile.status != "DONE":
            return false
    return true
```

---

## 8. Test Execution Protocol

### 8.1 Unit Tests

```bash
# Untuk setiap test scenario di task file:
go test -run Test{ScenarioName} ./internal/{domain}/
# Harus PASS
```

### 8.2 Integration Tests

```bash
# Untuk task yang melibatkan HTTP handler:
go test -run TestHandler ./internal/handler/
# Harus PASS
```

### 8.3 Test Failure Protocol

```
IF test gagal:
    1. STOP — jangan lanjut ke task berikutnya
    2. Update status: IN_PROGRESS → BLOCKED
    3. Tulis di file:
       ## Blocked Reason
       - Test failed: {scenario name}
       - Error: {error message}
       - Since: YYYY-MM-DD HH:MM
    4. Laporkan ke master agent
    5. Master agent akan assign fix task
```

### 8.4 Test Success Protocol

```
IF semua test lulus:
    1. Centang semua acceptance criteria (- [ ] → - [x])
    2. Update status: IN_PROGRESS → DONE
    3. Tulis Agent Log:
       ## Agent Log
       - Started: YYYY-MM-DD HH:MM
       - Completed: YYYY-MM-DD HH:MM
       - Agent: {agent-name}
       - All AC verified: ✓
       - All tests passed: ✓
```

---

## 9. Agent Team Structure

### 9.1 Domain Agents

| Agent ID | Domain | Tasks | Dependencies |
|----------|--------|-------|-------------|
| `agent-db` | Database | DB-001 → DB-015 | None |
| `agent-auth` | Auth | AUTH-001 → AUTH-015 | DB-001 |
| `agent-prov` | Providers | PROV-001 → PROV-015 | DB-001 |
| `agent-chat` | Chat/Core | CHAT-001 → CHAT-018 | DB-001, PROV |
| `agent-exec` | Executors | EXEC-001 → EXEC-015 | DB-001 |
| `agent-sys` | Settings | SYS-001 → SYS-016 | DB-004 |
| `agent-tunnel` | Tunnel | TUNNEL-001 → TUNNEL-012 | None |
| `agent-oauth` | OAuth | OAUTH-001 → OAUTH-014 | None |

### 9.2 Master Agent

```
Responsibilities:
  - Spawn domain agents
  - Monitor task statuses
  - Resolve blockers
  - Re-assign stuck tasks
  - Validate DONE claims
  - Maintain overall progress
```

---

## 10. Progress Tracking

### 10.1 Status Board

```
TODO:     ████████████████████████████████████ 130
IN_PROGRESS: ██ 0
DONE:     ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ 0
BLOCKED:  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ 0
```

### 10.2 Per-Domain Progress

```
AUTH:     TODO=15  IN_PROGRESS=0  DONE=0  BLOCKED=0
CHAT:     TODO=18  IN_PROGRESS=0  DONE=0  BLOCKED=0
COMBO:    TODO=10  IN_PROGRESS=0  DONE=0  BLOCKED=0
DB:       TODO=15  IN_PROGRESS=0  DONE=0  BLOCKED=0
EXEC:     TODO=15  IN_PROGRESS=0  DONE=0  BLOCKED=0
OAUTH:    TODO=14  IN_PROGRESS=0  DONE=0  BLOCKED=0
PROV:     TODO=15  IN_PROGRESS=0  DONE=0  BLOCKED=0
SYS:      TODO=16  IN_PROGRESS=0  DONE=0  BLOCKED=0
TUNNEL:   TODO=12  IN_PROGRESS=0  DONE=0  BLOCKED=0
```

### 10.3 Completion Signal

```
Untuk menandai task DONE:
  1. Update frontmatter: status: DONE
  2. Centang semua AC
  3. Tulis Agent Log lengkap
  4. Laporkan ke master agent
```

---

## 11. File Modification Rules

Agent HANYA boleh mengubah:

```
ALLOWED modifications:
  ✓ frontmatter status field (TODO → IN_PROGRESS → DONE → BLOCKED)
  ✓ Acceptance Criteria checkbox (- [ ] → - [x])
  ✓ Agent Log section (tambahan di bawah Logic)
  ✓ Blocked Reason section (jika BLOCKED)
  ✓ Completion section (jika DONE)

FORBIDDEN modifications:
  ✗ Mengubah Description
  ✗ Mengubah Input/Output
  ✗ Mengubah Logic
  ✗ Mengubah Acceptance Criteria text
  ✗ Mengubah Test Scenarios
  ✗ Menghapus bagian lain
```

---

## 12. Recovery / Resume Procedure

```
Jika agent crash atau restart:

1. Scan tasks/ untuk status=IN_PROGRESS
   → Cek Agent Log: Started timestamp
   → Jika >4 jam tanpa Completed → kembalikan ke TODO

2. Scan tasks/ untuk status=BLOCKED
   → Cek blocker dependencies
   → Jika dependency sudah DONE → kembalikan ke TODO

3. Lanjut dari task dengan status=TODO yang dependencies-nya DONE
```

---

## 13. Example: Complete Task Lifecycle

### 13.1 Initial State (DB-001)

```markdown
---
id: DB-001
domain: database
status: TODO
estimate: 1h
title: ProviderConnection Model
---

## Description
...

## Acceptance Criteria
- [ ] Struct compiles
- [ ] GORM AutoMigrate succeeds

## Test Scenarios
| ... |
```

### 13.2 Agent Claims (IN_PROGRESS)

```markdown
---
id: DB-001
domain: database
status: IN_PROGRESS        # ← diubah agent
estimate: 1h
title: ProviderConnection Model
---

## Description
...

## Logic
...

## Agent Log
- Started: 2026-06-04 16:00
- Agent: agent-db
```

### 13.3 Agent Verifies AC

```markdown
## Acceptance Criteria
- [x] Struct compiles               # ← dicentang
- [x] GORM AutoMigrate succeeds     # ← dicentang
```

### 13.4 Agent Completes (DONE)

```markdown
---
id: DB-001
domain: database
status: DONE                # ← diubah agent
estimate: 1h
title: ProviderConnection Model
---

## Description
...

## Acceptance Criteria
- [x] Struct compiles
- [x] GORM AutoMigrate succeeds

## Agent Log
- Started: 2026-06-04 16:00
- Completed: 2026-06-04 16:45
- Agent: agent-db
- AC-001 verified: go build ./internal/model/ passes
- AC-002 verified: gorm.AutoMigrate on sqlite :memory: succeeds

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/model/provider_connection.go
```

---

## 14. Quick Reference Card

```
UNTUK AGENT:

1. Baca task file (tasks/{ID}-{name}.md)
2. Cek status — harus TODO
3. Cek dependencies — semua harus DONE
4. Ganti status → IN_PROGRESS
5. Tulis Agent Log (Started)
6. Implement sesuai Logic
7. Jalankan test scenarios
8. Centang semua AC
9. Tulis Agent Log (Completed + evidence)
10. Ganti status → DONE

JANGAN:
- Lewati dependency check
- Skip test scenarios
- Tandai DONE tanpa AC terpenuhi
- Ubah konten selain status dan AC checkbox
```

---

## 15. Task Execution Order (Recommended)

```
Batch 1 (Parallel — no dependencies):
  ├─ DB-001...DB-015 (agent-db)
  ├─ TUNNEL-001...TUNNEL-012 (agent-tunnel)
  └─ OAUTH-001...OAUTH-014 (agent-oauth)

Batch 2 (After DB-001):
  ├─ AUTH-001...AUTH-015 (agent-auth)
  ├─ PROV-001...PROV-015 (agent-prov)
  └─ SYS-001...SYS-016 (agent-sys)

Batch 3 (After DB-001 + PROV):
  └─ CHAT-001...CHAT-018 (agent-chat)

Batch 4 (After DB-001 + CHAT):
  └─ EXEC-001...EXEC-015 (agent-exec)
```

---

*Document Version: 1.0*
*Last Updated: 2026-06-04*
*Author: Claude Code*
