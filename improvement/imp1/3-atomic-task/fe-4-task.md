# Atomic Task: fe-4 — Build PerKeyChart Component

**Domain**: Frontend
**Priority**: High
**Estimated effort**: 45 min

---

## Input

- `recharts` library (AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer)
- API endpoint: `GET /api/usage/per-key/[keyId]/chart?period=7d`
- Existing chart pattern from usage page

## Output

- `PerKeyChart` component embedded in `page.js`
- Area chart with Tokens/Cost toggle
- ResponsiveContainer, gradient fill, custom tooltip

## Process

### Step 1: Import recharts

```js
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
```

### Step 2: Add formatting helpers

```js
const fmtTokens = (n) => {
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
  return String(n || 0);
};
const fmtCost = (n) => `$${(n || 0).toFixed(2)}`;
```

### Step 3: Create PerKeyChart component

```jsx
function PerKeyChart({ keyId, period, chartData }) {
  const [data, setData] = useState(chartData || []);
  const [viewMode, setViewMode] = useState("tokens");

  useEffect(() => { setData(chartData || []); }, [chartData, period]);

  const fetchData = useCallback(async () => {
    try {
      const res = await fetch(`/api/usage/per-key/${keyId}/chart?period=${period}`);
      if (res.ok) setData((await res.json()).chartData);
    } catch {}
  }, [keyId, period]);

  useEffect(() => { fetchData(); }, [fetchData]);

  const hasData = data.some((d) => d.tokens > 0 || d.cost > 0);

  return (
    <Card className="flex min-w-0 flex-col gap-3 p-3 sm:p-4">
      {/* View mode toggle */}
      <div className="grid w-full grid-cols-2 items-center gap-1 rounded-lg border border-border bg-bg-subtle p-1 sm:w-auto sm:self-start">
        <button
          onClick={() => setViewMode("tokens")}
          className={`px-3 py-1 rounded-md text-sm font-medium transition-colors ${viewMode === "tokens" ? "bg-primary text-white shadow-sm" : "text-text-muted hover:text-text hover:bg-bg-hover"}`}
        >
          Tokens
        </button>
        <button
          onClick={() => setViewMode("cost")}
          className={`px-3 py-1 rounded-md text-sm font-medium transition-colors ${viewMode === "cost" ? "bg-primary text-white shadow-sm" : "text-text-muted hover:text-text hover:bg-bg-hover"}`}
        >
          Cost
        </button>
      </div>

      {!hasData ? (
        <div className="h-48 flex items-center justify-center text-text-muted text-sm">No data for this period</div>
      ) : (
        <ResponsiveContainer width="100%" height={220}>
          <AreaChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="gradTokens" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#6366f1" stopOpacity={0.25} />
                <stop offset="95%" stopColor="#6366f1" stopOpacity={0} />
              </linearGradient>
              <linearGradient id="gradCost" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.25} />
                <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" strokeOpacity={0.1} />
            <XAxis dataKey="label" tick={{ fontSize: 10, fill: "currentColor", fillOpacity: 0.5 }} tickLine={false} axisLine={false} interval="preserveStartEnd" />
            <YAxis tick={{ fontSize: 10, fill: "currentColor", fillOpacity: 0.5 }} tickLine={false} axisLine={false} tickFormatter={viewMode === "tokens" ? fmtTokens : fmtCost} width={50} />
            <Tooltip
              contentStyle={{ backgroundColor: "var(--color-bg)", border: "1px solid var(--color-border)", borderRadius: "8px", fontSize: "12px" }}
              formatter={(value, name) => [name === "tokens" ? fmtTokens(value) : fmtCost(value), name === "tokens" ? "Tokens" : "Cost"]}
            />
            {viewMode === "tokens" ? (
              <Area type="monotone" dataKey="tokens" stroke="#6366f1" strokeWidth={2} fill="url(#gradTokens)" dot={false} activeDot={{ r: 4 }} />
            ) : (
              <Area type="monotone" dataKey="cost" stroke="#f59e0b" strokeWidth={2} fill="url(#gradCost)" dot={false} activeDot={{ r: 4 }} />
            )}
          </AreaChart>
        </ResponsiveContainer>
      )}
    </Card>
  );
}
```

### Step 4: Add chart to page

```jsx
<PerKeyChart keyId={keyId} period={period} chartData={data.chartData} />
```

## Dependencies

- fe-2: PerKeyUsagePage shell (DONE)

## Success Criteria

- Chart renders below KPI cards
- Tokens/Cost toggle switches between two data views
- Gradient fills (indigo for tokens, amber for cost)
- Tooltip shows formatted values
- "No data for this period" shown when empty
- Chart re-fetches when period changes
- ResponsiveContainer adapts to container width
