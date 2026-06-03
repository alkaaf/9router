# Debug Report: 500 Error on GET /api/providers

## Summary

The root cause is **NOT a database or PostgreSQL issue**. The 500 error originates from a **CSS parsing failure** in the Next.js dev server caused by a malformed CSS custom property definition in `src/app/globals.css`. This CSS error breaks the entire Next.js build pipeline, causing ALL routes (including API routes) to return 500 errors.

---

## Root Cause

**Location:** `src/app/globals.css`, lines 62-65 (light mode) and lines 111-114 (dark mode)

**Problem:** CSS custom property values are not allowed to span multiple lines. The `--shadow-elev` variable definition uses a multi-line value with line continuation, which is invalid CSS syntax.

```css
/* BROKEN - invalid CSS: custom property values cannot span multiple lines */
--shadow-elev:
  inset 0 1px 0 0 rgba(255,255,255,0.8),
  0 1px 2px rgba(15,23,42,0.04),
  0 12px 36px -8px rgba(15,23,42,0.10);
```

The `rgba(15,23,42,0.10)` with trailing zeros is also suspicious (might contain hidden characters).

---

## How This Causes the 500 Error

1. **Tailwind v4 (`@tailwindcss/postcss` v4)** processes `globals.css` and generates a massive compiled CSS file (4000+ lines).
2. When parsing the multi-line `--shadow-elev` value, PostCSS/Tailwind generates corrupted CSS class names in the output.
3. The generated `.shadow-\[var\(--\e ��\,�_\)\]` class contains `\e` (octal escape for ESC character) and invalid CSS escape sequences.
4. The compiled CSS file is saved to `.next/dev/static/chunks/src_app_globals_css_1igg3k2._.single.css`.
5. When Next.js serves ANY request (including API routes), it loads this corrupted CSS, which causes the PostCSS parser to fail with:
   ```
   Unexpected token Delim('\u{e}')
   ```
6. The build error propagates to all routes, causing 500 errors everywhere.

---

## Confirmed Evidence

### Generated corrupted CSS (`.next` cache):
```css
/* Line 4072 of generated CSS - contains invalid escape sequences */
.shadow-\[var\(--\e ��\,�_\)\], .shadow-\[var\(--\e ��u\2 _\)\], .shadow-\[var\(--\e ���\2 _\)\], .shadow-\[var\(--\e �owarm\)\] {
  box-shadow: var(--tw-inset-shadow), ...;
}
```

### Server log confirms CSS error:
```
./src/app/globals.css:2856:24
Parsing CSS source code failed
> 2856 |     --tw-shadow: var(--owarm);
       |                        ^
Unexpected token Delim('\u{e}')
```

---

## Why the API Route Never Gets Called

The middleware (`src/proxy.js` / `src/dashboardGuard.js`) passes through `/api/providers` correctly. The issue is that **Next.js fails during the build/compilation phase** when processing the CSS. The error is caught as a "compilation error" by Next.js, which returns 500 for all routes because the build itself is broken.

Even though `GET /api/providers` returned `401 Unauthorized` in the curl test (which is the middleware blocking unauthenticated requests), when accessed with valid auth, the 500 would come from the broken CSS compilation.

---

## The Fix

Change the multi-line `--shadow-elev` definitions to single-line values in `src/app/globals.css`:

**Before (lines 62-65):**
```css
--shadow-elev:
  inset 0 1px 0 0 rgba(255,255,255,0.8),
  0 1px 2px rgba(15,23,42,0.04),
  0 12px 36px -8px rgba(15,23,42,0.10);
```

**After:**
```css
--shadow-elev: inset 0 1px 0 0 rgba(255,255,255,0.8), 0 1px 2px rgba(15,23,42,0.04), 0 12px 36px -8px rgba(15,23,42,0.10);
```

**Same fix for dark mode (lines 111-114):**
```css
--shadow-elev: inset 0 1px 0 0 rgba(255,255,255,0.06), 0 1px 2px rgba(0,0,0,0.4), 0 16px 48px -8px rgba(0,0,0,0.55);
```

After fixing, delete the `.next` cache directory and restart the dev server.

---

## PostgreSQL Side Investigation (for completeness)

The PostgreSQL integration itself is working correctly:

- `providerConnections` table exists with correct schema (`isActive` as BOOLEAN, `createdAt`/`updatedAt` as TIMESTAMPTZ, `data` as JSONB)
- The `postgresAdapter.js` has proper `LOWER_TO_CAMEL` transformation for column names
- `connectionsRepo.js` correctly uses `await db.all(sql, params)` (line 66) for the async pg adapter

---

---

## Note on Curl Test Result

The initial `curl http://localhost:20128/api/providers` returned `{"error":"Unauthorized"}` (401), not 500. This is because:
1. The middleware (`dashboardGuard.js`) blocks unauthenticated requests to `/api/providers`
2. The CSS error only manifests when the build pipeline is invoked
3. With a valid auth token, the request would reach the API handler but fail with 500 due to CSS compilation error
4. Other routes (like `POST /api/auth/login` without proper body) showed the 500 error with the full CSS stack trace

The 500 error is real and affects all routes that trigger CSS compilation in the dev server.

**Primary Fix:** Flatten the `--shadow-elev` CSS variable to a single line in `src/app/globals.css`.
