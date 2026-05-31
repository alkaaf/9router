# Atomic Task: fe-5 — Integrate UsageTable with Grouped Model Data

**Domain**: Frontend
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- `src/app/(dashboard)/dashboard/usage/components/UsageTable` — existing component
- `PerKeyUsagePage` from fe-2
- API response `data.byModel` (array of model entries)

## Output

- UsageTable rendered in PerKeyUsagePage
- Grouped by rawModel, sortable by requests/cost/tokens
- Custom summary and detail cells

## Process

### Step 1: Import UsageTable

```js
import UsageTable from "@/app/(dashboard)/dashboard/usage/components/UsageTable";
```

### Step 2: Add sorting state

```js
const [tableView, setTableView] = useState("model");
const [sortBy, setSortBy] = useState("requests");
const [sortOrder, setSortOrder] = useState("desc");
```

### Step 3: Create data processing memos

```js
const byModelEntries = useMemo(() => {
  if (!data?.byModel) return [];
  return Object.entries(data.byModel).map(([name, d]) => ({ name, ...d }));
}, [data]);

const sortedByModel = useMemo(() => {
  const sorted = [...byModelEntries];
  sorted.sort((a, b) => {
    const aVal = a[sortBy] ?? 0;
    const bVal = b[sortBy] ?? 0;
    return sortOrder === "asc" ? aVal - bVal : bVal - aVal;
  });
  return sorted;
}, [byModelEntries, sortBy, sortOrder]);

const groupedByModel = useMemo(() => {
  const map = {};
  for (const item of sortedByModel) {
    const rawModel = item.rawModel || item.name.split("|")[0];
    if (!map[rawModel]) map[rawModel] = { groupKey: rawModel, summary: { requests: 0, promptTokens: 0, completionTokens: 0, cost: 0, lastUsed: "" }, items: [] };
    map[rawModel].summary.requests += item.requests || 0;
    map[rawModel].summary.promptTokens += item.promptTokens || 0;
    map[rawModel].summary.completionTokens += item.completionTokens || 0;
    map[rawModel].summary.cost += item.cost || 0;
    if ((item.lastUsed || "") > (map[rawModel].summary.lastUsed || "")) map[rawModel].summary.lastUsed = item.lastUsed;
    map[rawModel].items.push(item);
  }
  return Object.values(map);
}, [sortedByModel]);
```

### Step 4: Add sort toggle handler

```js
const toggleSort = (field) => {
  setSortBy((prev) => {
    if (prev === field) {
      setSortOrder((o) => (o === "asc" ? "desc" : "asc"));
      return prev;
    }
    setSortOrder("desc");
    return field;
  });
};
```

### Step 5: Add fmtTime helper

```js
function fmtTime(ts) {
  if (!ts) return "—";
  const d = new Date(ts);
  const now = new Date();
  const diffMs = now - d;
  if (diffMs < 60000) return "Just now";
  if (diffMs < 3600000) return `${Math.floor(diffMs / 60000)}m ago`;
  if (diffMs < 86400000) return `${Math.floor(diffMs / 3600000)}h ago`;
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}
```

### Step 6: Add UsageTable to page

```jsx
<UsageTable
  title="Usage by Model"
  columns={[
    { field: "name", label: "Model" },
    { field: "provider", label: "Provider" },
  ]}
  groupedData={groupedByModel}
  tableType="model"
  sortBy={sortBy}
  sortOrder={sortOrder}
  onToggleSort={() => toggleSort(sortBy)}
  viewMode="costs"
  storageKey={`per-key:${keyId}:expanded-models`}
  renderSummaryCells={() => (
    <>
      <td className="px-6 py-3 text-text-muted">—</td>
      <td className="px-6 py-3 text-right">{fmt(groupedByModel.reduce((s, g) => s + g.summary.requests, 0))}</td>
      <td className="px-6 py-3 text-right text-text-muted whitespace-nowrap">{fmtTime(groupedByModel[0]?.summary.lastUsed)}</td>
    </>
  )}
  renderDetailCells={(item) => (
    <>
      <td className="px-6 py-3 font-medium">{item.rawModel || item.name}</td>
      <td className="px-6 py-3">{item.provider || "—"}</td>
      <td className="px-6 py-3 text-right">{fmt(item.requests)}</td>
      <td className="px-6 py-3 text-right text-text-muted whitespace-nowrap">{fmtTime(item.lastUsed)}</td>
    </>
  )}
  emptyMessage="No usage recorded for this key."
/>
```

Note: `fmt` helper is already defined in the file.

## Dependencies

- fe-2: PerKeyUsagePage shell (DONE)
- `UsageTable` component (existing, no changes)

## Success Criteria

- Table renders below chart
- Models grouped by rawModel name
- Click column headers to sort (asc/desc toggle)
- Summary row shows totals per group
- Detail rows show per-model breakdown
- `storageKey` persists expanded/collapsed state
- "No usage recorded" message when empty
