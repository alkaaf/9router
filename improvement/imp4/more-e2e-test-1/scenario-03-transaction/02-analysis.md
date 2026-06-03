# Scenario 03: Transaction Isolation — Analysis

## Result: **4 passed | 1 skipped (5 total)**

All 4 transaction tests passed. No failures.

## postgresAdapter.js — transaction() summary

`/Users/alkaaf/project/9router/src/lib/db/adapters/postgresAdapter.js` lines 198–212 (top-level) and 36–99 (per-txn adapter):

- **`transaction(fn)`** acquires a dedicated client from the pool, runs `BEGIN`, executes `fn(txAdapter)`, then `COMMIT` on success or `ROLLBACK` on error. The client is always released in `finally`.
- **`txAdapter`** (`createTransactionAdapter`) is a per-txn object that pins `run/get/all/exec` to the same checked-out client, so all queries inside the transaction share the BEGIN/COMMIT connection.
- **Nested transactions** (txn's own `transaction(fn)`) use Postgres `SAVEPOINT` / `RELEASE SAVEPOINT` / `ROLLBACK TO SAVEPOINT` with a random `sp_xxx` name (lines 68–82). This avoids Postgres's "already in transaction" error from nested `BEGIN`.
- **Return value** is forwarded — `await fn(txAdapter)`'s return value is what `db.transaction()` resolves with.

## Test coverage & results

| # | Test | What it verifies | Result |
|---|------|------------------|--------|
| 1 | transaction commits on success | UPDATE inside txn persists; visible after COMMIT | PASS (4ms) |
| 2 | transaction rolls back on error | UPDATE inside throwing txn is reverted to "before-rollback" | PASS (2ms) |
| 3 | nested transactions use SAVEPOINT | outer + inner (SAVEPOINT) both visible after commit | PASS (4ms) |
| 4 | transaction returns value from fn | `return "success-value"` is propagated out of `transaction()` | PASS (1ms) |

## Top root cause

**No root-cause bug.** The transaction implementation is correct for all four cases:
- `BEGIN`/`COMMIT` lifecycle wired correctly.
- `ROLLBACK` triggered on `fn` rejection.
- Savepoint naming is collision-safe (random suffix).
- Return value passthrough works.

The adapter's design (dedicated client + SAVEPOINT for nesting) matches the standard Postgres transaction model and matches what the tests assert.

## Notes / caveats

- The test does **not** cover concurrent writers or cross-transaction isolation levels (e.g., READ COMMITTED vs SERIALIZABLE), only the four basic transactional semantics listed in the task. Those would need additional cases.
- `[DB] Driver: postgres` printed once — driver selection works end-to-end.
- Duration 220ms total tests — fast, no pool exhaustion or retry loops.
