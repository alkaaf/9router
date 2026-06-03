# Debug Report: 500 Error on `GET /api/models/availability`

## Summary

The `GET /api/models/availability` endpoint returns a **500 Internal Server Error**. However, the root cause is **NOT** a PostgreSQL or database error. The entire Next.js dev server is broken due to a **CSS/Tailwind compilation failure** that prevents any route from responding normally.

---

## Root Cause

**File**: `src/app/globals.css` (and downstream Tailwind/PostCSS processing)

**Error**: CSS parsing failure at line 2856 of the PostCSS-transformed CSS output:
```
Unexpected token Delim('\u{e}')
.shadow-\[var\(--\e �owarm\)\] {
  --tw-shadow: var(-- owarm);
                       ^
```

This error cascades to ALL routes because Next.js cannot render the `_error` page (which itself imports `globals.css`), causing every endpoint to return 500 with the same CSS error body.

---

## Investigation Details

### 1. API Route Analysis

**File**: `/src/app/api/models/availability/route.js`

```javascript
export async function GET() {
  const connections = await getProviderConnections(); // → connectionsRepo.js
  // Returns: { models: [...], unavailableCount: N }
}
```

**Call chain**:
- `route.js` → `getProviderConnections()` (imported from `@/lib/localDb`)
- `@/lib/localDb.js` → re-exports from `@/lib/db/index.js`
- `@/lib/db/index.js` → barrel for repos, uses `@/lib/db/driver.js`
- `@/lib/db/driver.js` → uses PostgreSQL adapter via `createPostgresAdapter()`
- `postgresAdapter.js` → executes `SELECT * FROM providerConnections`

**Does it use `kv` table?** No. The availability endpoint only calls `getProviderConnections()` which reads `providerConnections` table. It does NOT access `kv`, `usageDaily`, or any other tables.

### 2. PostgreSQL Database Verification

Connected successfully. Verified:

| Table | Exists | Rows |
|-------|--------|------|
| `providerConnections` | YES | 0 |
| `kv` | YES | 5 |
| `settings` | YES | 1 |
| `usagedaily` | YES | 0 |
| `combos` | YES | 0 |

Schema is correct — all columns present (`id`, `provider`, `authType`, `name`, `email`, `priority`, `isActive`, `data`, `createdAt`, `updatedAt`). The `transformRowKeys()` function in `postgresAdapter.js` properly converts lowercase pg column names back to camelCase.

### 3. Direct Adapter Test

```javascript
// Verified: PostgreSQL adapter works correctly
adapter.all('SELECT * FROM providerConnections LIMIT 5')
// Returns: [] (empty, correct — no data in test DB)
```

### 4. The Actual Blockers

**Blocker 1**: Auth returns 401 without CLI token
- `/api/models/*` requires auth (see `dashboardGuard.js` PROTECTED_API_PATHS)
- Fix: Use `x-9r-cli-token` header (value: `a00b470be698d1bc`)

**Blocker 2**: CSS compilation failure (the actual 500 cause)

The CSS file `src/app/globals.css` is only **497 lines**, but the error references **line 2856** of the PostCSS-generated output. The problem occurs during Tailwind's CSS processing of `shadow-[var(--shadow-warm)]` class.

Components using this pattern (confirmed via grep):
- `src/shared/components/Card.js` — `shadow-[var(--shadow-elev)]`
- `src/shared/components/Drawer.js` — `shadow-[var(--shadow-elev)]`
- `src/shared/components/Modal.js` — `shadow-[var(--shadow-elev)]`
- `src/shared/components/NineRemotePromoModal.js` — `shadow-[var(--shadow-warm)]`
- `src/shared/components/Sidebar.js` — `shadow-[var(--shadow-warm)]`
- `src/shared/components/ThemeToggle.js` — `shadow-[var(--shadow-warm)]`
- `src/app/(dashboard)/dashboard/skills/page.js` — `shadow-[var(--shadow-soft)]`

Tailwind CSS v4 (`@tailwindcss/postcss: ^4.1.18`, `tailwindcss: ^4`) with `@tailwindcss/postcss` plugin appears to have an issue processing the `shadow-[var(--shadow-WARM)]` arbitrary value syntax when it contains multi-part CSS variable names.

The PostCSS-generated CSS shows the class name was transformed to `shadow-\[var\(--\e �owarm\)\]` — the `--shadow-warm` was parsed as `\` + `e` + `owarm` (with control character artifacts), and the `--` prefix became `--\e` (where `\e` is an invalid escape).

### 5. Why All Endpoints Return 500

Even public endpoints like `/api/health`, `/api/init`, `/api/auth/status` return 500 with the same CSS error. This happens because:

1. Any route error causes Next.js to render `_error.jsx`
2. The error page imports `globals.css` (via `src/app/layout.js`)
3. The CSS compilation fails (Turbopack cannot process the CSS)
4. The error page itself fails to render
5. Final response: 500 with the CSS error body

---

## Conclusion

The 500 error is a **CSS/Tailwind v4 compilation failure**, NOT a database issue. The PostgreSQL integration is working correctly:

- Driver: `postgres` (confirmed via logs: `[DB] Driver: postgres`)
- Adapter: `createPostgresAdapter()` initializes successfully
- Schema: All tables (`providerConnections`, `kv`, `settings`, etc.) exist and are properly typed
- Queries: `SELECT * FROM providerConnections` works and returns correct column names via `transformRowKeys()`

**The DB is NOT the problem.** The fix is to resolve the Tailwind CSS arbitrary value parsing issue with `shadow-[var(--shadow-*)]` patterns.

### Possible Fixes

1. **Quick fix**: Replace `shadow-[var(--shadow-warm)]` with standard Tailwind shadow utilities or use CSS classes instead of arbitrary values referencing CSS variables
2. **Update Tailwind config**: Ensure `shadow-*` arbitrary values are properly supported in v4
3. **Check postcss.config.mjs**: The current config uses `@tailwindcss/postcss` with `base: projectRoot` — verify this is the correct configuration for Tailwind v4

---

## Key Files

| File | Role |
|------|------|
| `/src/app/api/models/availability/route.js` | API endpoint |
| `/src/lib/db/repos/connectionsRepo.js` | `getProviderConnections()` |
| `/src/lib/db/adapters/postgresAdapter.js` | PostgreSQL driver adapter |
| `/src/lib/db/driver.js` | DB initialization, chooses postgres if `POSTGRES_URL` set |
| `/src/lib/db/schema.postgres.js` | PostgreSQL DDL schema |
| `/src/app/globals.css` | Global styles (where CSS error originates) |
| `/src/proxy.js` + `/src/dashboardGuard.js` | Auth middleware |
| `/src/shared/utils/machineId.js` | CLI token generation |
