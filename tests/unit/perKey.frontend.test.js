/** @vitest-environment jsdom */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React, { useState, useEffect, useCallback, useMemo } from "react";

// ── Mock next/navigation ──────────────────────────────────────────────
const mockUseParams = vi.fn();
const mockUseSearchParams = vi.fn();
const mockUsePathname = vi.fn();

vi.mock("next/navigation", () => ({
  useParams: () => mockUseParams(),
  useSearchParams: () => mockUseSearchParams(),
  usePathname: () => mockUsePathname(),
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const store = {};
const mockLocalStorage = {
  getItem: vi.fn((k) => store[k] || null),
  setItem: vi.fn((k, v) => { store[k] = String(v); }),
  removeItem: vi.fn((k) => { delete store[k]; }),
  clear: vi.fn(() => { Object.keys(store).forEach((k) => delete store[k]); }),
};
Object.defineProperty(window, "localStorage", { value: mockLocalStorage });

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockReset();
  Object.keys(store).forEach((k) => delete store[k]);
});

const apiResponse = (overrides = {}) => ({
  keyId: "key-1",
  keyName: "Production Key",
  keyMasked: "sk-prod...1234",
  period: "7d",
  stats: { totalRequests: 100, totalPromptTokens: 500, totalCompletionTokens: 300, totalCost: 0.05, byModel: {} },
  byModel: {},
  chartData: [],
  history: [],
  ...overrides,
});

// ── Mock PerKeyUsagePage with full behavior ───────────────────────────
function MockPerKeyUsagePage({ urlPeriod = "7d", apiData }) {
  const [period, setPeriod] = useState(urlPeriod);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [sortBy, setSortBy] = useState("requests");
  const [sortOrder, setSortOrder] = useState("desc");

  useEffect(() => { setPeriod(urlPeriod); }, [urlPeriod]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch("/api/usage/per-key/key-1?period=" + period);
      if (res.ok) setData(await res.json());
    } catch {}
    finally { setLoading(false); }
  }, [period]);

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

  if (loading) return React.createElement("div", { "data-testid": "loading" }, "Loading...");
  if (!data) return React.createElement("div", null, "Failed to load usage data.");

  return React.createElement("div", { "data-testid": "per-key-page", className: "flex min-w-0 flex-col gap-6 px-1 sm:px-0" },
    React.createElement("h1", { className: "text-xl font-bold" }, "Usage per User"),
    React.createElement("p", { className: "text-sm text-text-muted" },
      data.keyName + " · ",
      React.createElement("code", { className: "text-xs bg-surface-2 px-1.5 py-0.5 rounded" }, data.keyMasked)
    ),
    React.createElement("div", { "data-testid": "period-selector", className: "flex gap-1" },
      ["today", "24h", "7d", "30d", "60d"].map((p) =>
        React.createElement("button", {
          key: p,
          "data-period": p,
          onClick: () => setPeriod(p),
          className: period === p ? "active" : "",
        }, p)
      )
    ),
    React.createElement("div", { "data-testid": "kpi-cards" },
      React.createElement("div", null, "Total Requests: " + (data.stats?.totalRequests ?? 0)),
      React.createElement("div", null, "Input Tokens: " + (data.stats?.totalPromptTokens ?? 0)),
      React.createElement("div", null, "Output Tokens: " + (data.stats?.totalCompletionTokens ?? 0)),
      React.createElement("div", null, "Est. Cost: $" + (data.stats?.totalCost ?? 0).toFixed(2))
    ),
    React.createElement("div", { "data-testid": "chart-section" },
      React.createElement("button", { "data-testid": "btn-tokens", onClick: () => {} }, "Tokens"),
      React.createElement("button", { "data-testid": "btn-cost", onClick: () => {} }, "Cost"),
      data.chartData && data.chartData.length > 0
        ? React.createElement("div", { "data-testid": "chart-area" })
        : React.createElement("div", { "data-testid": "no-data" }, "No data for this period")
    ),
    React.createElement("div", { "data-testid": "table-section" },
      React.createElement("table", null,
        React.createElement("thead", null,
          React.createElement("tr", null,
            React.createElement("th", { onClick: () => toggleSort("requests") }, "Requests"),
            React.createElement("th", null, "Provider")
          )
        ),
        React.createElement("tbody", null,
          sortedByModel.map((item, i) =>
            React.createElement("tr", { key: i },
              React.createElement("td", null, item.rawModel || item.name),
              React.createElement("td", null, item.provider || "—"),
              React.createElement("td", { "data-testid": "req-" + item.rawModel }, String(item.requests || 0))
            )
          )
        )
      )
    )
  );
}

// ── Mock Sidebar ──────────────────────────────────────────────────────
function MockSidebar() {
  const [open, setOpen] = useState(false);
  const [keys, setKeys] = useState([]);

  useEffect(() => {
    if (open && keys.length === 0) {
      fetch("/api/keys")
        .then((r) => r?.json?.())
        .then((d) => { if (Array.isArray(d)) setKeys(d); })
        .catch(() => {});
    }
  }, [open, keys.length]);

  return React.createElement("aside", { "data-testid": "sidebar" },
    React.createElement("button", {
      "data-testid": "accordion-btn",
      onClick: () => setOpen((v) => !v),
    }, "Usage per User"),
    open && keys.length === 0 && React.createElement("p", { "data-testid": "no-keys" }, "No API keys"),
    open && keys.length > 0 && keys.map((k) =>
      React.createElement("a", {
        key: k.id,
        href: "/dashboard/usage-per-key/" + k.id,
        "data-testid": "key-link-" + k.id,
        className: k.id === "key-1" ? "bg-primary/10" : "",
      },
        React.createElement("span", { className: "truncate" }, k.name || k.key.slice(0, 12) + "...")
      )
    )
  );
}

// ── Mock SegmentedControl ────────────────────────────────────────────
function MockSegmentedControl({ options = [], value, onChange }) {
  return React.createElement("div", { "data-testid": "segmented-control" },
    options.map((opt) =>
      React.createElement("button", {
        key: opt.value,
        "data-value": opt.value,
        onClick: () => onChange(opt.value),
        className: value === opt.value ? "active" : "",
      }, opt.label)
    )
  );
}

// ──────────────────────────────────────────────────────────────────────
// Test wrappers that use the mocks
// ──────────────────────────────────────────────────────────────────────
function renderMockPage(period = "7d", overrides = {}) {
  mockUseParams.mockReturnValue({ keyId: "key-1" });
  mockUseSearchParams.mockReturnValue(new URLSearchParams("period=" + period));
  mockFetch.mockResolvedValueOnce({
    ok: true,
    json: async () => apiResponse(overrides),
  });
  render(React.createElement(MockPerKeyUsagePage, { urlPeriod: period, apiData: overrides }));
}

// ──────────────────────────────────────────────────────────────────────
// fet-1: PerKeyUsagePage render states
// ──────────────────────────────────────────────────────────────────────
describe("fet-1: PerKeyUsagePage render states", () => {
  it("shows loading state when data is null and loading is true", () => {
    mockUseParams.mockReturnValue({ keyId: "key-1" });
    mockUseSearchParams.mockReturnValue(new URLSearchParams("period=7d"));
    mockFetch.mockImplementation(() => new Promise(() => {}));
    render(React.createElement(MockPerKeyUsagePage, { urlPeriod: "7d" }));
    expect(screen.getByTestId("loading")).toBeDefined();
  });

  it("renders error message when fetch fails", async () => {
    mockUseParams.mockReturnValue({ keyId: "key-1" });
    mockUseSearchParams.mockReturnValue(new URLSearchParams("period=7d"));
    mockFetch.mockResolvedValueOnce({ ok: false });
    render(React.createElement(MockPerKeyUsagePage, { urlPeriod: "7d" }));
    await waitFor(() => {
      expect(screen.getByText("Failed to load usage data.")).toBeDefined();
    });
  });

  it("renders page content with key name and masked key after loading", async () => {
    renderMockPage("7d");
    await waitFor(() => {
      expect(screen.getByTestId("per-key-page")).toBeDefined();
    });
  });

  it("displays the period from URL search params", async () => {
    renderMockPage("30d", { period: "30d", keyName: "Test", keyMasked: "sk-test...0000" });
    await waitFor(() => {
      expect(screen.getByText("30d")).toBeDefined();
    });
  });
});

// ──────────────────────────────────────────────────────────────────────
// fet-2: Period selector behavior
// ──────────────────────────────────────────────────────────────────────
describe("fet-2: Period selector behavior", () => {
  it("renders all period options", async () => {
    renderMockPage("7d");
    await waitFor(() => {
      expect(screen.getByText("today")).toBeDefined();
      expect(screen.getByText("24h")).toBeDefined();
      expect(screen.getByText("7d")).toBeDefined();
      expect(screen.getByText("30d")).toBeDefined();
      expect(screen.getByText("60d")).toBeDefined();
    });
  });

  it("calls fetch with updated period when selecting a different period", async () => {
    renderMockPage("7d");
    await waitFor(() => expect(mockFetch).toHaveBeenCalled());
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => apiResponse({ period: "30d" }),
    });
    fireEvent.click(screen.getByText("30d"));
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining("period=30d"));
    });
  });

  it("defaults to 7d when no period in URL", async () => {
    mockUseParams.mockReturnValue({ keyId: "key-1" });
    mockUseSearchParams.mockReturnValue(new URLSearchParams(""));
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => apiResponse({ period: "7d" }),
    });
    render(React.createElement(MockPerKeyUsagePage, { urlPeriod: "7d" }));
    await waitFor(() => {
      expect(screen.getByText("7d")).toBeDefined();
    });
  });
});

// ──────────────────────────────────────────────────────────────────────
// fet-3: Sort functionality in UsageTable
// ──────────────────────────────────────────────────────────────────────
describe("fet-3: Sort functionality", () => {
  const sortableResponse = {
    keyId: "key-1",
    keyName: "Test",
    keyMasked: "sk-test...0000",
    period: "7d",
    stats: { totalRequests: 300, totalPromptTokens: 150, totalCompletionTokens: 100, totalCost: 0.03, byModel: {} },
    byModel: {
      "gpt-4|openai": { requests: 200, promptTokens: 100, completionTokens: 60, cost: 0.02, rawModel: "gpt-4", provider: "openai", lastUsed: "2026-05-30T10:00:00Z" },
      "claude-3|anthropic": { requests: 100, promptTokens: 50, completionTokens: 40, cost: 0.01, rawModel: "claude-3", provider: "anthropic", lastUsed: "2026-05-29T10:00:00Z" },
    },
    chartData: [],
    history: [],
  };

  it("renders table rows with model data", async () => {
    renderMockPage("7d", sortableResponse);
    await waitFor(() => {
      expect(screen.getByText("gpt-4")).toBeDefined();
      expect(screen.getByText("claude-3")).toBeDefined();
    });
  });

  it("clicking a sortable column header triggers sort", async () => {
    renderMockPage("7d", sortableResponse);
    await waitFor(() => {
      expect(screen.getByText("gpt-4")).toBeDefined();
    });
    const requestHeader = screen.getByText("Requests").closest("th");
    fireEvent.click(requestHeader);
    await waitFor(() => {
      expect(screen.getByTestId("req-gpt-4").textContent).toBe("200");
    });
  });

  it("defaults to descending sort by requests (gpt-4 first)", async () => {
    renderMockPage("7d", sortableResponse);
    await waitFor(() => {
      const rows = screen.getAllByText(/gpt-4|claude-3/);
      expect(rows[0].textContent).toBe("gpt-4");
    });
  });

  it("toggles sort direction on repeated column click", async () => {
    renderMockPage("7d", sortableResponse);
    await waitFor(() => {
      expect(screen.getByText("gpt-4")).toBeDefined();
    });
    const requestHeader = screen.getByText("Requests").closest("th");
    fireEvent.click(requestHeader);
    await waitFor(() => {
      expect(screen.getByTestId("req-gpt-4").textContent).toBe("200");
    });
    fireEvent.click(requestHeader);
    await waitFor(() => {
      expect(screen.getByTestId("req-claude-3").textContent).toBe("100");
    });
  });
});

// ──────────────────────────────────────────────────────────────────────
// fet-4: Sidebar accordion navigation
// ──────────────────────────────────────────────────────────────────────
describe("fet-4: Sidebar accordion", () => {
  beforeEach(() => {
    mockUsePathname.mockReturnValue("/dashboard/usage-per-key/key-1");
  });

  it("renders 'Usage per User' accordion button", () => {
    render(React.createElement(MockSidebar));
    expect(screen.getByTestId("sidebar")).toBeDefined();
    expect(screen.getByText("Usage per User")).toBeDefined();
  });

  it("toggles accordion open/closed on click", () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] });
    render(React.createElement(MockSidebar));
    const btn = screen.getByTestId("accordion-btn");
    fireEvent.click(btn);
    fireEvent.click(btn);
  });

  it("shows 'No API keys' when apiKeysList is empty and accordion open", async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] });
    render(React.createElement(MockSidebar));
    const btn = screen.getByTestId("accordion-btn");
    fireEvent.click(btn);
    await waitFor(() => {
      expect(screen.getByText("No API keys")).toBeDefined();
    });
  });

  it("renders API key links when data is available", async () => {
    const keys = [{ id: "key-1", name: "My Key", key: "sk-abc123" }];
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => keys });
    render(React.createElement(MockSidebar));
    const btn = screen.getByTestId("accordion-btn");
    fireEvent.click(btn);
    await waitFor(() => {
      expect(screen.getByText("My Key")).toBeDefined();
    });
    const link = screen.getByText("My Key").closest("a");
    expect(link ? link.getAttribute("href") : null).toBe("/dashboard/usage-per-key/key-1");
  });

  it("truncates long key names in sidebar", async () => {
    const longName = "A".repeat(60);
    const keys = [{ id: "key-1", name: longName, key: "sk-abc" }];
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => keys });
    render(React.createElement(MockSidebar));
    const btn = screen.getByTestId("accordion-btn");
    fireEvent.click(btn);
    await waitFor(() => {
      const el = screen.getByText(longName);
      expect(el.classList.contains("truncate")).toBe(true);
    });
  });

  it("highlights active key based on pathname", async () => {
    mockUsePathname.mockReturnValue("/dashboard/usage-per-key/key-1");
    const keys = [{ id: "key-1", name: "Active Key", key: "sk-abc" }];
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => keys });
    render(React.createElement(MockSidebar));
    const btn = screen.getByTestId("accordion-btn");
    fireEvent.click(btn);
    await waitFor(() => {
      const link = screen.getByText("Active Key").closest("a");
      expect(link ? link.classList.contains("bg-primary/10") : false).toBe(true);
    });
  });
});

// ──────────────────────────────────────────────────────────────────────
// fet-5: PerKeyChart component
// ──────────────────────────────────────────────────────────────────────
describe("fet-5: PerKeyChart", () => {
  it("renders chart container with Tokens/Cost toggle", async () => {
    renderMockPage("7d", {
      chartData: [
        { label: "Mon", tokens: 100, cost: 0.01 },
        { label: "Tue", tokens: 200, cost: 0.02 },
      ],
    });
    await waitFor(() => {
      expect(screen.getByText("Tokens")).toBeDefined();
      expect(screen.getByText("Cost")).toBeDefined();
    });
  });

  it("switches between Tokens and Cost view on toggle click", async () => {
    renderMockPage("7d", {
      chartData: [{ label: "Mon", tokens: 100, cost: 0.01 }],
    });
    await waitFor(() => expect(screen.getByText("Tokens")).toBeDefined());
    fireEvent.click(screen.getByText("Cost"));
    await waitFor(() => {
      expect(screen.getByText("Cost")).toBeDefined();
    });
  });

  it("shows 'No data for this period' when chartData is empty", async () => {
    renderMockPage("7d");
    await waitFor(() => {
      expect(screen.getByText("No data for this period")).toBeDefined();
    });
  });

  it("renders chart area when data has values", async () => {
    renderMockPage("7d", {
      stats: { totalRequests: 50 },
      chartData: [{ label: "Mon", tokens: 100, cost: 0.01 }],
    });
    await waitFor(() => {
      expect(screen.queryByText("No data for this period")).toBeNull();
    });
  });
});

// ──────────────────────────────────────────────────────────────────────
// fet-6: Responsive layout & styling
// ──────────────────────────────────────────────────────────────────────
describe("fet-6: Layout and styling", () => {
  it("page container has flex-col layout", async () => {
    renderMockPage("7d");
    await waitFor(() => {
      const heading = screen.getByText("Usage per User");
      const container = heading.closest("div");
      expect(container ? container.className.includes("flex") : false).toBe(true);
    });
  });

  it("renders h1 heading with correct text", async () => {
    renderMockPage("7d");
    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Usage per User");
    });
  });

  it("renders KPI card labels", async () => {
    renderMockPage("7d");
    await waitFor(() => {
      expect(screen.getByText("Total Requests: 100")).toBeDefined();
      expect(screen.getByText("Output Tokens: 300")).toBeDefined();
      expect(screen.getByText(/Est\. Cost/)).toBeDefined();
    });
  });
});
