# Combos Domain — Atomic Task Breakdown

**Domain:** Combos (model groups with load-balancing strategies)
**Parent manifest:** `/Users/alkaaf/project/9router/improvement/imp5/manifest.md` (v1.2)
**Target stack:** Go + Fiber + GORM (SQLite/PostgreSQL)
**Source files covered:**
- `src/app/api/combos/route.js` (list/create)
- `src/app/api/combos/[id]/route.js` (read/update/delete)
- `open-sse/services/combo.js` (rotation state + combo chat handler)
- `src/sse/handlers/chat.js` (combo detection + strategy resolution)
- `src/lib/db/repos/combosRepo.js` (DB-layer CRUD)
- `src/lib/db/schema.js` (table definition)

---

## 1. Domain Overview

A **Combo** is a named bundle of model strings (`["gpt-4o", "claude-sonnet-4-5", "gemini-2.5-pro"]`) that the chat handler treats as a single virtual model. When a client sends `model: "my-combo"`, the gateway iterates the list and:

- **Fallback** (default): try model 1; on a `shouldFallback` error, try model 2; …
- **Round-robin**: rotate the starting index per request so load spreads; respects a *sticky limit* (default 1) so a given combo entry is hit N times in a row before advancing.
- **Per-combo override**: a combo may pin its own strategy in `settings.comboStrategies[<name>].fallbackStrategy`, overriding the global `settings.comboStrategy`.

Runtime state is **per-process, in-memory** (a `Map<comboName, {index, consecutiveUseCount}>`). Any create/update/delete on a combo (or rename) must reset the relevant entry, otherwise rotation indices go stale.

### Current SQL schema (`combos` table)
```
id        TEXT PRIMARY KEY
name      TEXT UNIQUE NOT NULL
kind      TEXT
models    TEXT NOT NULL  -- JSON array of model strings
createdAt TEXT NOT NULL
updatedAt TEXT NOT NULL
```

### Current HTTP contract
| Method | Path | Auth | Behavior |
|--------|------|------|----------|
| GET    | `/api/combos`        | JWT/CLI | List all combos |
| POST   | `/api/combos`        | JWT/CLI | Create combo (name regex `^[a-zA-Z0-9_.\-]+$`, unique) |
| GET    | `/api/combos/[id]`   | JWT/CLI | Read single |
| PUT    | `/api/combos/[id]`   | JWT/CLI | Update (validates name + uniqueness, resets rotation) |
| DELETE | `/api/combos/[id]`   | JWT/CLI | Delete (resets rotation) |

### Critical edge cases observed
- Combo with **one model** skips rotation entirely (`models.length <= 1`).
- `stickyLimit` ≤ 0 / non-numeric falls back to **1**.
- 503/502/504 transient errors: **wait** `cooldownMs` (≤ 5 s) before falling through to next model.
- "No credentials" error → final 503 (not 406).
- Earliest `retryAfter` across combo models is surfaced on final failure.
- Combo lookup must skip when model string contains `/` (provider/model form).

---

## 2. Dependency Graph (top-level)

```
COMBO-001  Combo GORM model
   │
   ▼
COMBO-002  Combo repository
   │
   ├──────────────────────────────────────────┐
   ▼                                          ▼
COMBO-010  GET /api/combos         COMBO-020  Strategy interface
COMBO-011  POST /api/combos                 │
COMBO-012  GET /api/combos/[id]             ├─→ COMBO-021  Fallback strategy
COMBO-013  PUT /api/combos/[id]             ├─→ COMBO-022  Round-robin strategy
COMBO-014  DELETE /api/combos/[id]          └─→ COMBO-023  Sticky-session strategy
   │                                          │
   ▼                                          ▼
COMBO-030  Combo validation helpers     COMBO-040  Rotation state manager
                                            │
                                            ▼
                                       COMBO-050  Settings-driven strategy resolver
                                            │
                                            ▼
                                       COMBO-060  Chat-handler integration (replaces
                                                  open-sse/services/combo.js)
                                            │
                                            ▼
                                       COMBO-070  Combo chat handler
                                            │
                                            ▼
                                       COMBO-080  Streaming fallback (carry SSE across
                                                  model retries)
```

Tests are interleaved (`*_test.go`) and treated as part of each task's "Test strategy" so reviewers see coverage alongside design.

---

## 3. Atomic Tasks

### COMBO-001 — Define Combo GORM Model
- **Description:** Create `internal/model/combo.go` with the `Combo` struct mirroring the SQLite `combos` table (id TEXT PK, name TEXT UNIQUE NOT NULL, kind *string, models TEXT NOT NULL storing JSON array, createdAt/updatedAt time.Time). Provide typed accessors `GetModels() ([]string, error)` and `SetModels([]string) error` that marshal/unmarshal the `models` JSON column. Use `gorm:"type:text"` and an `uniqueIndex` on Name to match the existing constraint.
- **Input/Output Contract:**
  - **In:** none (compile-time definition)
  - **Out:** `model.Combo` struct with GORM tags, satisfies `BeforeCreate` hook generating a UUID v4 ID when empty
- **Test Strategy:** Unit tests in `internal/model/combo_test.go`:
  - `GetModels` returns empty slice for empty string, original slice for round-trip.
  - `SetModels` produces valid JSON; rejects nil/empty without panic.
  - `BeforeCreate` sets a non-empty `ID`.
  - `AutoMigrate(&Combo{})` against in-memory SQLite creates the table with the unique index.
- **Dependencies:** none
- **Phase:** 1.3 (Database Layer)

### COMBO-002 — Combo Repository
- **Description:** Create `internal/repository/combo.go` exposing `ComboRepository` with `FindAll() ([]model.Combo, error)`, `FindByID(id string) (*model.Combo, error)`, `FindByName(name string) (*model.Combo, error)`, `Create(*model.Combo) error`, `Update(id string, fields map[string]any) (*model.Combo, error)`, `Delete(id string) (bool, error)`. Match the Node.js behavior in `combosRepo.js`: list is `ORDER BY createdAt ASC`; updates merge existing row with supplied fields and bump `updatedAt`; deletes return whether a row was affected.
- **Input/Output Contract:**
  - **In:** repository methods receive primitive args; `Update` accepts a `map[string]any` of partial fields.
  - **Out:** GORM result + domain error, with `gorm.ErrRecordNotFound` translated to `nil` entity (no sentinel — the handler decides 404).
- **Test Strategy:** `internal/repository/combo_test.go` against `:memory:` SQLite:
  - Seed three combos, `FindAll` returns them in `createdAt ASC` order.
  - `FindByName` returns the right row; case sensitivity preserved.
  - `Update` with partial fields preserves untouched columns.
  - `Delete` returns `(true, nil)` for existing, `(false, nil)` for missing.
- **Dependencies:** COMBO-001
- **Phase:** 1.3

### COMBO-003 — Settings schema fields (combo-related)
- **Description:** Extend `model.Setting.Data` JSON schema (settings lives in `settings` table as a single JSON blob) with combo-related fields and document them: `comboStrategy string` (default `"fallback"`), `comboStickyRoundRobinLimit int` (default `1`), `comboStrategies map[string]ComboStrategyOverride` where `ComboStrategyOverride{FallbackStrategy string}`. Validate these fields inside the existing `internal/handler/api/settings.go` PATCH handler — but the **combo handler** also reads them, so expose a typed `ComboSettings` accessor in the repository.
- **Input/Output Contract:**
  - **In:** raw settings JSON from DB.
  - **Out:** `settings.GetComboSettings()` returns a fully-typed `ComboSettings` struct, filling defaults for missing fields.
- **Test Strategy:** Unit test against seeded settings rows:
  - Empty settings → all defaults applied.
  - Partial settings (e.g. `comboStrategy: "round-robin"` only) → other fields default.
  - Malformed `comboStrategies` map does not panic.
- **Dependencies:** COMBO-002 (reads from same `Settings` repo)
- **Phase:** 2.1 (Core CRUD)

### COMBO-010 — GET /api/combos handler
- **Description:** Fiber handler `ListCombos` in `internal/handler/api/combos.go` that calls `comboRepo.FindAll()` and returns `{"combos": [...]}` with status 200. Use the shared `response.JSON` helper. Errors return `500 { "error": "Failed to fetch combos" }` matching the Node.js message.
- **Input/Output Contract:**
  - **In:** HTTP GET, no body
  - **Out 200:** `{"combos":[{"id":"...","name":"...","kind":...,"models":[],"createdAt":"...","updatedAt":"..."},...]}`
  - **Out 500:** `{"error":"Failed to fetch combos"}`
- **Test Strategy:** Integration test in `internal/handler/api/combos_test.go`:
  - Empty DB → 200 with `combos: []`.
  - Seeded 3 combos → 200 with all 3 in `createdAt ASC` order.
  - Simulated DB error → 500 with error message.
- **Dependencies:** COMBO-002, auth middleware (Phase 1.4)
- **Phase:** 2.1

### COMBO-011 — POST /api/combos handler
- **Description:** Fiber handler `CreateCombo` that:
  1. Parses JSON body `{name, models, kind}`.
  2. Returns 400 with `"Name is required"` if `name` missing.
  3. Validates `name` against `^[a-zA-Z0-9_.\-]+$`; on failure returns 400 with the Node.js message `"Name can only contain letters, numbers, -, _ and ."`.
  4. Calls `comboRepo.FindByName(name)`; if found returns 400 with `"Combo name already exists"`.
  5. Persists via `Create` and returns 201 with the new combo (not wrapped).
  6. On unexpected error returns 500 `"Failed to create combo"`.
- **Input/Output Contract:**
  - **In:** `{"name":"my-combo","models":["gpt-4o","claude-sonnet-4-5"],"kind":null}`
  - **Out 201:** the new combo object
  - **Out 400:** `{"error":"<message>"}` (three distinct messages)
  - **Out 500:** `{"error":"Failed to create combo"}`
- **Test Strategy:** Table-driven test:
  - Missing name → 400 `"Name is required"`.
  - Invalid name (contains space, slash, unicode) → 400 with regex message.
  - Duplicate name → 400 `"Combo name already exists"`.
  - Valid request with empty models → 201, persisted as `[]`.
  - Valid request with `kind` → persisted; `kind` returned.
  - DB error during insert → 500.
- **Dependencies:** COMBO-002, COMBO-010
- **Phase:** 2.1

### COMBO-012 — GET /api/combos/[id] handler
- **Description:** Fiber handler `GetCombo` that resolves `:id` via `c.Params("id")`, calls `FindByID`, and:
  - Returns 200 with the combo object on hit.
  - Returns 404 `{"error":"Combo not found"}` on miss.
  - Returns 500 `{"error":"Failed to fetch combo"}` on error.
- **Input/Output Contract:**
  - **In:** URL param `id`
  - **Out 200:** the combo object
  - **Out 404:** `{"error":"Combo not found"}`
  - **Out 500:** `{"error":"Failed to fetch combo"}`
- **Test Strategy:** Integration test:
  - Existing ID → 200 with body match.
  - Non-existent ID → 404 with exact message.
  - Malformed ID (still passes FindByID) → 404.
- **Dependencies:** COMBO-002
- **Phase:** 2.1

### COMBO-013 — PUT /api/combos/[id] handler
- **Description:** Fiber handler `UpdateCombo` that:
  1. Reads `id` and JSON body.
  2. If `body.name` is present: validates regex; calls `FindByName(body.name)` and returns 400 `"Combo name already exists"` if a different combo already owns the name.
  3. Loads the current row (`prev`) to know the prior name.
  4. Calls `Update(id, body)`; if `nil` is returned (record not found) → 404 `"Combo not found"`.
  5. **Rotation invalidation:** calls `comboRotation.Reset(prev.Name)` AND, if renamed, `comboRotation.Reset(combo.Name)`. (This is the only consumer of COMBO-040 in the handler layer.)
  6. Returns 200 with the updated combo.
- **Input/Output Contract:**
  - **In:** `id` from URL, body may include `name`, `models`, `kind`.
  - **Out 200:** updated combo object
  - **Out 400:** regex/uniqueness errors
  - **Out 404:** `"Combo not found"`
  - **Out 500:** `"Failed to update combo"`
- **Test Strategy:** Integration test (with mock rotation manager):
  - Update models only → rotation for prev name reset; new name not reset.
  - Rename → rotation reset for both old and new names.
  - Conflict name → 400 without mutating DB.
  - Missing id → 404.
- **Dependencies:** COMBO-002, COMBO-040
- **Phase:** 2.1

### COMBO-014 — DELETE /api/combos/[id] handler
- **Description:** Fiber handler `DeleteCombo` that:
  1. Loads `prev` to capture the name.
  2. Calls `Delete(id)`; if `(false, nil)` → 404 `"Combo not found"`.
  3. Calls `comboRotation.Reset(prev.Name)`.
  4. Returns 200 `{"success": true}`.
- **Input/Output Contract:**
  - **In:** URL param `id`
  - **Out 200:** `{"success":true}`
  - **Out 404:** `{"error":"Combo not found"}`
  - **Out 500:** `{"error":"Failed to delete combo"}`
- **Test Strategy:** Integration test:
  - Existing id → 200 + rotation reset for that name.
  - Missing id → 404.
  - Subsequent GET /api/combos/[id] → 404 (verify cascade).
- **Dependencies:** COMBO-002, COMBO-040
- **Phase:** 2.1

### COMBO-020 — Strategy interface
- **Description:** Create `internal/combo/strategy.go` with:
  ```go
  type Selector interface {
      // NextOrder returns the models in the order they should be tried.
      NextOrder(comboName string, models []string) []string
      // Reset clears any per-combo state for comboName (no-op for fallback).
      Reset(comboName string)
      // ResetAll clears all state.
      ResetAll()
  }
  ```
  Plus a `Kind` enum: `KindFallback`, `KindRoundRobin`. `NewSelector(kind string, stickyLimit int) (Selector, error)` returns the right implementation or an error for unknown kinds. `stickyLimit ≤ 0` is normalized to `1` (matching `normalizeStickyLimit` in combo.js).
- **Input/Output Contract:**
  - **In:** strategy kind string, sticky limit (any numeric).
  - **Out:** `Selector` ready for use; error if kind is unknown.
- **Test Strategy:** Unit tests in `internal/combo/strategy_test.go`:
  - `NewSelector("fallback", 3)` returns FallbackSelector.
  - `NewSelector("round-robin", 0)` normalizes sticky to 1.
  - `NewSelector("nonsense", 1)` returns error.
- **Dependencies:** none
- **Phase:** 3.8

### COMBO-021 — Fallback strategy
- **Description:** Implement `FallbackSelector` in `internal/combo/fallback.go`. `NextOrder` returns `models` unchanged. `Reset` and `ResetAll` are no-ops. Behaviour must match the Node.js fallback: try index 0, then 1, … in input order.
- **Input/Output Contract:**
  - **In:** `models []string`.
  - **Out:** same slice (or a defensive copy — pick a copy to keep the contract safe for mutating callers).
- **Test Strategy:**
  - Returns input order for arbitrary slices.
  - No internal state: 1000 calls return identical results.
- **Dependencies:** COMBO-020
- **Phase:** 3.8

### COMBO-022 — Round-robin strategy
- **Description:** Implement `RoundRobinSelector` in `internal/combo/roundrobin.go`. Use a `sync.Map[string, *rrState]` (Go 1.21+ `sync.Map` with typed wrapper) keyed by `comboName` (default `"__default__"`), where `rrState = struct{ index int; consecutiveUseCount int }`. On each `NextOrder`:
  1. If `len(models) <= 1`, return `models` unchanged.
  2. Read state (or seed `{0,0}`).
  3. Compute `rotated = rotateSlice(models, state.index)`.
  4. Increment `consecutiveUseCount`; if `>= stickyLimit` → advance `index` and reset counter; else keep `index` and store new counter.
  5. Persist updated state.
  6. Return `rotated`.
- **Input/Output Contract:**
  - **In:** `comboName string`, `models []string`.
  - **Out:** rotated `[]string` (new slice).
- **Test Strategy:** Unit tests with table-driven rotation:
  - 1 model → unchanged, no state stored.
  - 3 models, sticky=1 → first call `[a,b,c]`, second `[b,c,a]`, third `[c,a,b]`, fourth `[a,b,c]`.
  - 3 models, sticky=3 → first three calls all return `[a,b,c]`, fourth returns `[b,c,a]`.
  - Sticky=0 → coerced to 1.
  - Different combo names maintain independent indices.
  - Concurrent calls from 100 goroutines: invariant `index ∈ [0, len)` always holds; final index correct modulo 100.
- **Dependencies:** COMBO-020, COMBO-040
- **Phase:** 3.8

### COMBO-023 — Sticky-session strategy
- **Description:** Implement `StickySelector` in `internal/combo/sticky.go` that wraps `RoundRobinSelector` but maintains stickiness at the **session** level. Session key derives from a request-scoped `SessionKey` interface (e.g. `apiKey` + `clientIP`). For the initial port, the chat handler passes `comboStickyRoundRobinLimit` from settings, and the sticky selector sticks to the same `models[0]` for that many requests before rotating. Internally store `map[comboName]map[sessionKey]int` and look up using the session key passed via a `NextOrderWithSession` variant.
- **Input/Output Contract:**
  - **In:** `comboName string`, `models []string`, `sessionKey string`.
  - **Out:** rotated `[]string` (a one-element shift that puts the sticky model first).
- **Test Strategy:**
  - sticky=3, models=[a,b,c] → first 3 calls return `[a,b,c]`, `[a,b,c]`, `[a,b,c]`.
  - After session switch, first 3 calls return `[b,c,a]`, etc. (independent counter).
  - `Reset(comboName)` clears only that combo.
  - `ResetAll()` clears everything.
- **Dependencies:** COMBO-022
- **Phase:** 3.8
- **Notes:** Spec wording is ambiguous between "sticky round-robin" (the chat.js default `comboStickyRoundRobinLimit`) and "sticky sessions". The Node.js code uses the former; this task implements the **session-keyed** version (true sticky) for forward-compatibility. If the simpler form is wanted instead, mark as out-of-scope and use RoundRobinSelector with a `stickyLimit` from settings.

### COMBO-030 — Combo name validation helpers
- **Description:** Create `internal/combo/validate.go` with `IsValidName(name string) bool` implementing `^[a-zA-Z0-9_.\-]+$`. Exported so the handler tests can call it directly. Also expose `NormalizeModels(models []string) []string` (trims whitespace, drops empty entries, de-dupes preserving order).
- **Input/Output Contract:**
  - **In:** raw string / slice
  - **Out:** bool / normalized slice
- **Test Strategy:** Table-driven:
  - Valid: `a`, `a-b_c.d`, `1.2.3`, `model.42`
  - Invalid: `""`, `a/b`, `a b`, `a$b`, `a@b`
  - Normalize: `["a"," a ","","b"]` → `["a","b"]`.
- **Dependencies:** none
- **Phase:** 2.1

### COMBO-040 — Rotation state manager
- **Description:** Create `internal/combo/rotation.go` with a `RotationManager` interface `Reset(name string)` / `ResetAll()`, plus a default `InMemoryRotation` implementation backed by `sync.RWMutex`-protected `map[string]*rrState`. This is the singleton injected into the API handler and the strategy implementations. Expose `Snapshot() map[string]rrState` for testability and `/api/internal/rotation` debug endpoint (out of scope for combos, but the method must be present).
- **Input/Output Contract:**
  - **In:** combo name (or empty for `ResetAll`).
  - **Out:** side effect on internal map; `Snapshot` returns a copy.
- **Test Strategy:** Concurrency tests:
  - Concurrent `Reset` and `ResetAll` are race-free (`go test -race`).
  - `Reset` on missing key is a no-op.
  - `Snapshot` returns deep copy (mutating it does not affect internal state).
- **Dependencies:** none
- **Phase:** 3.8

### COMBO-050 — Settings-driven strategy resolver
- **Description:** Create `internal/combo/resolver.go` with `Resolve(comboName string, settings model.ComboSettings) (Selector, error)`. Logic mirrors `chat.js` lines 95-100:
  ```
  perCombo = settings.ComboStrategies[comboName].FallbackStrategy
  kind     = perCombo ?? settings.ComboStrategy ?? "fallback"
  ```
  Sticky limit is `settings.ComboStickyRoundRobinLimit` (default 1). The resolver caches `Selector` instances per (kind, stickyLimit) pair — but the underlying `RoundRobinSelector` still keeps the per-combo rotation state, so caching is safe.
- **Input/Output Contract:**
  - **In:** combo name, settings struct.
  - **Out:** `Selector` ready to call.
- **Test Strategy:**
  - No per-combo override + global `"fallback"` → FallbackSelector.
  - Global `"round-robin"` → RoundRobinSelector with sticky=1.
  - Per-combo override wins over global.
  - Empty per-combo + global round-robin + sticky=3 → round-robin with sticky=3.
- **Dependencies:** COMBO-020, COMBO-021, COMBO-022, COMBO-003
- **Phase:** 3.8

### COMBO-060 — Chat-handler integration: combo detection
- **Description:** In `internal/handler/v1/chat.go` (and the streaming variant), before resolving a model, call `comboRepo.FindByName(body.Model)`. If found and `combo.Models` is non-empty, dispatch to the combo chat flow. If `body.Model` contains `/`, **skip** the combo lookup (matches `getComboModelsFromData` behaviour). The dispatch path must:
  1. Load `settings.GetComboSettings()`.
  2. Call `combo.Resolve(model, settings)` to get a `Selector`.
  3. Call `selector.NextOrder(comboName, combo.Models)` to get the ordered model list.
  4. Forward to `handleComboChat` (COMBO-070).
- **Input/Output Contract:**
  - **In:** OpenAI-format chat request body.
  - **Out:** delegates to COMBO-070 with the resolved model order.
- **Test Strategy:** Handler test (table-driven) with mocked combo repo:
  - `model: "gpt-4o"` → existing alias flow, combo path skipped.
  - `model: "my-combo"` → combo path invoked with `["gpt-4o","claude-sonnet-4-5"]`.
  - `model: "unknown"` → 400 Invalid model format.
  - Empty combo models list → 400 Invalid model format.
- **Dependencies:** COMBO-002, COMBO-050
- **Phase:** 3.8

### COMBO-070 — Combo chat handler
- **Description:** Port `handleComboChat` from `open-sse/services/combo.js` to `internal/combo/handler.go`. Signature:
  ```go
  func HandleChat(ctx context.Context, req HandleChatRequest) (*HandleChatResult, error)
  type HandleChatRequest struct {
      Body            json.RawMessage
      Models          []string
      HandleSingle    func(ctx context.Context, body json.RawMessage, model string) (*SingleResult, error)
      Log             Logger
      ComboName       string
      Strategy        string
      StickyLimit     int
  }
  type SingleResult struct {
      Response   *http.Response
      Status     int
      StatusText string
      Ok         bool
  }
  ```
  Behaviour:
  - Iterate `rotatedModels`; for each call `HandleSingle`.
  - On `Ok` → return result.
  - Else parse error JSON, compute `{shouldFallback, cooldownMs}` via `internal/auth/fallback.go`'s `CheckFallbackError`.
  - Track earliest `retryAfter` across all models.
  - For transient 502/503/504 with `cooldownMs ≤ 5000` → `time.Sleep(cooldownMs)` then continue.
  - On `!shouldFallback` → return result immediately (preserves the failing response).
  - On exhausted loop → 503 with retry-after; message "no credentials" maps to 503.
- **Input/Output Contract:**
  - **In:** ordered models + single-model callback.
  - **Out:** the first successful response, or final 503.
- **Test Strategy:** Unit + integration:
  - All succeed → first model wins, no iterations past it.
  - First fails 429, second succeeds → second returned, state advances.
  - All fail with `"no credentials"` → 503.
  - First 503 transient with cooldown 100 ms → second invoked after at least 100 ms.
  - First 401 (no fallback) → return first response unchanged.
  - Earliest retry-after preserved when all fail.
- **Dependencies:** COMBO-050, COMBO-080
- **Phase:** 3.8

### COMBO-080 — Streaming fallback across model retries
- **Description:** When a combo model fails **mid-stream** (after headers are already flushed), the client already has an open SSE channel. The handler must:
  1. Detect mid-stream failure by checking the executor's `Stream(...)` channel for a non-`Ok` terminal event.
  2. Decide whether the error is fallback-eligible (`checkFallbackError`).
  3. If yes, open a **fresh** SSE channel for the next model — but the original HTTP response is already committed. Two approaches:
     - **Buffer-then-flush:** consume the new model's stream and tee it to the wire (preferred, lower client churn).
     - **Reset stream:** close the existing stream with an error SSE event and start a new one. Acceptable for clients that handle stream restarts.
  4. Track cumulative usage tokens and final `finish_reason` across retries; the final chunk carries the last model's `finish_reason`, but the response `usage` block is summed.
  Choose the **buffer-then-flush** variant; it matches the Node.js behaviour where `handleSingleModel` returns a fully-built `Response` object. Expose `OpenSSEResetHandler` and `OpenSSEBufferHandler` as separate strategies if the executor supports both.
- **Input/Output Contract:**
  - **In:** first model, list of fallback models, request body.
  - **Out:** single SSE response on the wire, carrying the chosen model's stream.
- **Test Strategy:** Integration test with two mock providers (httptest):
  - Provider 1: emits 2 chunks then a 500-trailing error.
  - Provider 2: emits 3 chunks then a `finish_reason: stop`.
  - Client sees: 2 chunks (or 1) + flush of provider 2's 3 chunks + `[DONE]`. Total chunks = 5.
  - Usage: prompt/completion tokens = sum of both.
- **Dependencies:** COMBO-070
- **Phase:** 4.2

### COMBO-090 — Settings migration: defaults
- **Description:** When bootstrapping a fresh DB, ensure the `settings` row contains the combo defaults (`comboStrategy: "fallback"`, `comboStickyRoundRobinLimit: 1`, `comboStrategies: {}`). Implement as part of `repository.Settings.UpsertDefaults` (Phase 1.3) so the chat handler can rely on these fields existing.
- **Input/Output Contract:**
  - **In:** existing or empty settings row.
  - **Out:** row with combo defaults populated.
- **Test Strategy:** Integration test:
  - Fresh DB → `GetSettings()` returns combo defaults.
  - Existing row with `comboStrategy: "round-robin"` → that value preserved.
- **Dependencies:** COMBO-003
- **Phase:** 2.1

### COMBO-100 — Contract test: byte-for-byte parity with Node.js
- **Description:** Record Node.js responses for the following fixtures and assert Go matches:
  - `GET /api/combos` (empty + 3 seeded).
  - `POST /api/combos` (success, duplicate, invalid name).
  - `PUT /api/combos/[id]` (rename, models change).
  - `DELETE /api/combos/[id]`.
  - `POST /v1/chat/completions` with combo model "my-combo" — first model 429, second model success.
- **Input/Output Contract:**
  - **In:** JSON fixtures under `testdata/contract/combos/`.
  - **Out:** pass/fail; on fail, diff Node vs Go body.
- **Test Strategy:** Standard contract test pattern (Phase 6.4 of manifest). 12 fixtures total.
- **Dependencies:** COMBO-010..014, COMBO-070
- **Phase:** 6.4

---

## 4. Out of Scope / Future Work

- **MITM combo routing.** The MITM server may also need combo awareness for tools that hit `/v1/chat/completions` directly. Currently the Node.js MITM proxies back to `/v1/chat/completions`, so the combo logic still runs. If MITM starts short-circuiting, this becomes a follow-up.
- **Per-combo cooldown persistence.** Currently rotation state is in-memory. A restart resets it. If that's a problem, persist `Map<comboName, rrState>` in the `kv` table with TTL.
- **Combo warmup/preflight.** No equivalent exists in Node.js either, so not part of this rewrite.
- **Combo metrics.** Expose per-combo success/fail counts via `/metrics` for Prometheus scraping. The Node.js code does not track this; deferred to the post-migration phase.

---

## 5. Acceptance Criteria

1. All 5 REST endpoints (`GET/POST /api/combos`, `GET/PUT/DELETE /api/combos/[id]`) return identical status codes and JSON shapes to the Node.js implementation.
2. Combo name validation rejects `/` and whitespace; allows `.`, `-`, `_`.
3. Round-robin rotation: with `sticky=1` and models `[a,b,c]`, the Nth request starts at index `N mod 3`; with `sticky=3` it advances every 3 requests.
4. Per-combo strategy override beats global.
5. `PUT /api/combos/[id]` invalidates the rotation state for both old and new names when renaming.
6. `DELETE /api/combos/[id]` invalidates rotation state.
7. Streaming combo chat with a mid-stream first-model failure produces a single coherent SSE response drawn from the second model, with usage tokens summed.
8. Contract tests (COMBO-100) pass with 0 byte-level diffs against recorded Node.js responses.
9. Unit-test coverage on `internal/combo` and `internal/handler/api/combos.go` is ≥ 95 % (manifest target).
10. E2E test (Playwright): create a combo in the dashboard, send a chat request, observe the response.

---

## 6. Suggested Implementation Order

1. COMBO-001, COMBO-002, COMBO-003 (data layer)
2. COMBO-010, COMBO-011, COMBO-012, COMBO-013, COMBO-014 (CRUD; can ship in one PR)
3. COMBO-020, COMBO-021, COMBO-040 (strategy foundation; Fallback is trivial)
4. COMBO-022, COMBO-050, COMBO-030 (round-robin + resolver)
5. COMBO-060, COMBO-070 (chat integration + handler)
6. COMBO-080 (streaming fallback)
7. COMBO-023, COMBO-090, COMBO-100 (polish, settings defaults, contract tests)

This ordering mirrors the dependency graph in §2 and unblocks the dashboard UI team after step 2 without waiting for step 5.
