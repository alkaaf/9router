"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { useParams, useSearchParams } from "next/navigation";
import { CardSkeleton, SegmentedControl, Card } from "@/shared/components";
import OverviewCards from "@/app/(dashboard)/dashboard/usage/components/OverviewCards";
import UsageTable from "@/app/(dashboard)/dashboard/usage/components/UsageTable";
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";

const PERIODS = [
  { value: "today", label: "Today" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7D" },
  { value: "30d", label: "30D" },
  { value: "60d", label: "60D" },
];

const fmt = (n) => new Intl.NumberFormat().format(n || 0);
const fmtCost = (n) => `$${(n || 0).toFixed(2)}`;
const fmtTokens = (n) => {
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
  return String(n || 0);
};

export default function PerKeyUsagePage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const keyId = params.keyId;

  const urlPeriod = searchParams.get("period") || "7d";
  const [period, setPeriod] = useState(urlPeriod);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [tableView, setTableView] = useState("model");
  const [sortBy, setSortBy] = useState("requests");
  const [sortOrder, setSortOrder] = useState("desc");

  useEffect(() => { setPeriod(urlPeriod); }, [urlPeriod]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`/api/usage/per-key/${keyId}?period=${period}`);
      if (res.ok) setData(await res.json());
    } catch {}
    finally { setLoading(false); }
  }, [keyId, period]);

  useEffect(() => { fetchData(); }, [fetchData]);

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

  if (loading) return <CardSkeleton />;
  if (!data) return <div className="text-text-muted text-sm">Failed to load usage data.</div>;

  return (
    <div className="flex min-w-0 flex-col gap-6 px-1 sm:px-0">
      {/* Header */}
      <div className="flex flex-col gap-1">
        <h1 className="text-xl font-bold">Usage per User</h1>
        <p className="text-sm text-text-muted">
          {data.keyName || "Unnamed Key"} &middot; <code className="text-xs bg-surface-2 px-1.5 py-0.5 rounded">{data.keyMasked}</code>
        </p>
      </div>

      {/* Period selector */}
      <SegmentedControl options={PERIODS} value={period} onChange={setPeriod} className="w-full sm:w-auto" />

      {/* KPI Cards */}
      <OverviewCards stats={data.stats} />

      {/* Chart */}
      <PerKeyChart keyId={keyId} period={period} chartData={data.chartData} />

      {/* Table */}
      {tableView === "model" && (
        <UsageTable
          title="Usage by Model"
          columns={[
            { field: "name", label: "Model" },
            { field: "provider", label: "Provider" },
            { field: "requests", label: "Requests", align: "right" },
            { field: "lastUsed", label: "Last Used", align: "right" },
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
      )}
    </div>
  );
}

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
            <XAxis
              dataKey="label"
              tick={{ fontSize: 10, fill: "currentColor", fillOpacity: 0.5 }}
              tickLine={false}
              axisLine={false}
              interval="preserveStartEnd"
            />
            <YAxis
              tick={{ fontSize: 10, fill: "currentColor", fillOpacity: 0.5 }}
              tickLine={false}
              axisLine={false}
              tickFormatter={viewMode === "tokens" ? fmtTokens : fmtCost}
              width={50}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: "var(--color-bg)",
                border: "1px solid var(--color-border)",
                borderRadius: "8px",
                fontSize: "12px",
              }}
              formatter={(value, name) =>
                name === "tokens" ? [fmtTokens(value), "Tokens"] : [fmtCost(value), "Cost"]
              }
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
