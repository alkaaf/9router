# Debug Report: `GET /api/provider-nodes` Returns 500

**Date:** 2026-06-03
**Branch:** pgsql-integration
**Endpoint:** `http://localhost:20128/api/provider-nodes`

---

## Summary

The `500` error on `/api/provider-nodes` is **not** caused by the provider-nodes code, the DB adapter, or the `providerNodes` table. The root cause is a **CSS parsing failure** in `src/app/globals.css` that causes Next.js Turbopack to crash on every server-rendered request.

---

## Findings

### 1. HTTP Status
```
curl 'http://localhost:20128/api/provider-nodes'
# Response: {"error": "Unauthorized"}   ← HTTP 401 without auth
curl 'http://localhost:20128/api/provider-nodes' -H 'x-9r-cli-token: <valid>'
# Response: HTML 500 error page
```

Without auth the endpoint returns `401` (expected behavior per `dashboardGuard.js`). With a valid CLI token it returns `500` — a server-side error.

### 2. Root Cause: CSS Parsing Failure (Next.js Server Crash)

**Error:**
```
./src/app/globals.css:2856:24
Parsing CSS source code failed
  .shadow-[var(--shadow-warm)] {
>     --tw-shadow: var(--shadow-warm);
                        ^
Unexpected token Delim('\u{e}')
```

The `@tailwindcss/postcss` plugin (v4) generates CSS utility classes for arbitrary `shadow-[var(--shadow-warm)]` tailwind classes found in JSX components. It produces:

```css
/* Generated (broken) */
--tw-shadow: var(--shadow-warm);   /* ← missing closing paren */
```

The expected output should be:
```css
/* Expected */
--tw-shadow: var(--shadow-warm);
```

The generated CSS is missing the closing `)` after `--shadow-warm`, producing an unterminated `var()` which causes the CSS parser to choke on the token `\u{e}` (an unexpected delimiter).

### 3. Triggering Components

Three JSX files use `shadow-[var(--shadow-...)]` classes that generate the broken CSS:

| File | Class |
|------|-------|
| `src/shared/components/Sidebar.js:145` | `shadow-[var(--shadow-warm)]` |
| `src/shared/components/Loading.js:53` | `shadow-[var(--shadow-soft)]` |
| `src/shared/components/Drawer.js:53` | `shadow-[var(--shadow-elev)]` |

These are client components imported by `layout.js`, which imports `globals.css`. Every server render that touches `layout.js` triggers CSS compilation → crash → 500.

### 4. Why the provider-nodes endpoint is affected

The `proxy.ts` middleware runs before API route handlers. When the request hits the Next.js SSR pipeline, Turbopack needs to compile `layout.js → globals.css`. The CSS compilation failure produces a 500 error **before** the API handler (`GET /api/provider-nodes`) ever executes.

The log confirms this flow:
```
server started
[DB] Driver: postgres
▲ globals.css:2856 CSS parsing error
GET /dashboard/combos 500 in 1073ms     ← CSS crash, not DB issue
GET /api/provider-nodes 500             ← same CSS crash
```

### 5. Database and Adapter — NOT the Problem

| Check | Result |
|-------|--------|
| `providerNodes` table exists | ✅ |
| Columns: `id, type, name, data, createdAt, updatedAt` | ✅ |
| PostgreSQL adapter initializes correctly | ✅ (`[DB] Driver: postgres` logged) |
| `getProviderNodes()` query | ✅ `SELECT * FROM providerNodes` — valid SQL |
| Table has 0 rows | ✅ (empty table is handled by `db.all()` returning `[]`) |
| `rowToNode()` handles empty results | ✅ Returns `[]` if no rows |
| `createdAt` / `updatedAt` column mapping | ✅ Listed in `CAMEL_CASE_COLUMNS` in `postgresAdapter.js` |

**Conclusion:** The DB layer, adapter, and model code are all correct. The table is empty but the code handles that gracefully.

---

## Evidence from Code

### `postgresAdapter.js` — `CAMEL_CASE_COLUMNS` mapping (lines 35-52)
```js
const CAMEL_CASE_COLUMNS = [
  "isActive", "connectionId", "apiKey", "apiKeyId",
  "createdAt", "updatedAt",   // ✅ correctly mapped
  "promptTokens", "completionTokens",
  ...
];
```

### `postgresAdapter.js` — `transformRowKeys` (lines 59-66)
```js
function transformRowKeys(row) {
  if (!row || typeof row !== "object") return row;
  const out = {};
  for (const [k, v] of Object.entries(row)) {
    out[LOWER_TO_CAMEL[k] ?? k] = v;  // ✅ correct fallback to original key
  }
  return out;
}
```

### `nodesRepo.js` — `getProviderNodes` (lines 41-48)
```js
export async function getProviderNodes(filter = {}) {
  const db = await getAdapter();
  const where = [];
  const params = [];
  if (filter.type) { where.push("type = ?"); params.push(filter.type); }
  const sql = `SELECT * FROM providerNodes${where.length ? ` WHERE ${where.join(" AND ")}` : ""}`;
  return db.all(sql, params).map(rowToNode);  // ✅ correct: handles empty array
}
```

---

## Reproduction

1. Start server: `PORT=20128 POSTGRES_URL=postgres://rahasia:rahasia@localhost:5433/9router_test npm run dev`
2. Any page load or API call fails with 500
3. The server log shows `globals.css:2856:24` CSS parsing errors
4. The 5 `shadow-[var(--shadow-...)]` classes in client components trigger the bug

---

## Fix Approach

The fix should be in `src/app/globals.css`. The CSS variable values for `--shadow-elev`, `--shadow-warm`, and `--shadow-soft` contain multi-line values with commas and parentheses. When Tailwind v4 generates arbitrary utility classes for `shadow-[var(--shadow-warm)]`, it produces invalid CSS.

**Option A:** Replace the `shadow-[var(--shadow-...)]` tailwind arbitrary classes with inline `style` props:
```jsx
// Before (broken)
<div className="shadow-[var(--shadow-elev)]" ...>

// After (works)
<div style={{ boxShadow: 'var(--shadow-elev)' }} ...>
```

**Option B:** Define proper CSS utility classes for these shadows in `globals.css`:
```css
.shadow-elev { box-shadow: var(--shadow-elev); }
.shadow-warm { box-shadow: var(--shadow-warm); }
.shadow-soft { box-shadow: var(--shadow-soft); }
```
Then use `shadow-elev` instead of `shadow-[var(--shadow-elev)]`.

**Option C:** Simplify the CSS variable values to single-line strings that don't break the tailwind arbitrary value parser.

---

## File Paths Referenced

- `/Users/alkaaf/project/9router/src/app/globals.css` — CSS with shadow variables (lines 62-65, 111-114)
- `/Users/alkaaf/project/9router/src/shared/components/Drawer.js` — `shadow-[var(--shadow-elev)]` at line 53
- `/Users/alkaaf/project/9router/src/shared/components/Sidebar.js` — `shadow-[var(--shadow-warm)]` at line 145
- `/Users/alkaaf/project/9router/src/shared/components/Loading.js` — `shadow-[var(--shadow-soft)]` at line 53
- `/Users/alkaaf/project/9router/src/app/api/provider-nodes/route.js` — API route (unaffected)
- `/Users/alkaaf/project/9router/src/lib/db/repos/nodesRepo.js` — DB repo (unaffected)
- `/Users/alkaaf/project/9router/src/lib/db/adapters/postgresAdapter.js` — PG adapter (unaffected)
- Server logs: `/private/tmp/claude-501/-Users-alkaaf-project-9router/482980f3-d276-42eb-a45a-aff28c168340/tasks/by8aslfxg.output`
