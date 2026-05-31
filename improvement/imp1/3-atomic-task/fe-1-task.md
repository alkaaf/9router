# Atomic Task: fe-1 — Add "Usage per User" Accordion to Sidebar

**Domain**: Frontend
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- `src/shared/components/Sidebar.js` — existing sidebar with accordion pattern (Media Providers)
- API endpoint: `GET /api/keys` (existing, returns list of API keys)
- Route pattern: `/dashboard/usage-per-key/[keyId]`

## Output

- Sidebar extended with "Usage per User" accordion section
- Accordion expands to show list of API keys
- Each key links to its per-key usage page
- Accordion is highlighted when user is on a per-key page

## Process

### Step 1: Add state variables

In the `Sidebar` component, add:

```js
const [usagePerUserOpen, setUsagePerUserOpen] = useState(false);
const [apiKeysList, setApiKeysList] = useState([]);
```

### Step 2: Add fetch effect

```js
useEffect(() => {
  if (usagePerUserOpen && apiKeysList.length === 0) {
    fetch("/api/keys")
      .then(res => res.json())
      .then(data => setApiKeysList(data.keys || []))
      .catch(() => {});
  }
}, [usagePerUserOpen, apiKeysList.length]);
```

### Step 3: Add accordion UI (in navItems section, before System)

```jsx
{/* Usage per User accordion */}
<div className="mb-1">
  <button
    onClick={() => setUsagePerUserOpen(!usagePerUserOpen)}
    className={`flex items-center gap-2 w-full px-3 py-2 rounded-lg text-sm transition-colors ${
      pathname.startsWith("/dashboard/usage-per-key")
        ? "bg-bg-hover text-text font-medium"
        : "text-text-muted hover:text-text hover:bg-bg-hover"
    }`}
  >
    <span className="material-symbols-outlined text-[18px]">monitoring</span>
    <span className="text-[13px] font-medium flex-1 text-left">Usage per User</span>
    <span className="material-symbols-outlined text-[14px] transition-transform"
      style={{ transform: usagePerUserOpen ? "rotate(180deg)" : "rotate(0deg)" }}>
      expand_more
    </span>
  </button>
  {usagePerUserOpen && (
    <div className="ml-4 mt-1 space-y-0.5">
      {apiKeysList.length === 0 ? (
        <span className="text-xs text-text-muted px-3 py-1 block">No API keys</span>
      ) : (
        apiKeysList.map(k => (
          <a
            key={k.id}
            href={`/dashboard/usage-per-key/${k.id}`}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-xs transition-colors ${
              pathname.startsWith(`/dashboard/usage-per-key/${k.id}`)
                ? "bg-bg-hover text-text font-medium"
                : "text-text-muted hover:text-text hover:bg-bg-hover"
            }`}
          >
            <span className="truncate">{k.name || "Unnamed Key"}</span>
            <span className="text-text-muted truncate">{k.key.slice(0, 8)}...</span>
          </a>
        ))
      )}
    </div>
  )}
</div>
```

### Step 4: Verify icon exists

Ensure `monitoring` icon is available in the Material Symbols font used by the sidebar.

## Dependencies

- None (independent — can be done in parallel with other frontend tasks)

## Success Criteria

- Accordion shows "Usage per User" with monitoring icon
- Click toggles expand/collapse
- On first expand, fetches `/api/keys` and renders list
- Each key shows name + masked key, links to `/dashboard/usage-per-key/[keyId]`
- Active key highlighted when on its page
- Accordion cached — second expand doesn't re-fetch
