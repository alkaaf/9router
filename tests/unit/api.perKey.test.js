import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

let tempDir;
const originalDataDir = process.env.DATA_DIR;

beforeEach(() => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "9router-api-"));
  process.env.DATA_DIR = tempDir;
  delete global._dbAdapter;
  vi.resetModules();
});

afterEach(() => {
  try { global._dbAdapter?.instance?.close?.(); } catch {}
  delete global._dbAdapter;
  if (tempDir) fs.rmSync(tempDir, { recursive: true, force: true });
  if (originalDataDir === undefined) delete process.env.DATA_DIR;
  else process.env.DATA_DIR = originalDataDir;
});

function seedDb(db) {
  const keyA = "sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaAA";
  const keyB = "sk-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbBB";
  db.run(`INSERT INTO apiKeys(id, key, name, machineId, isActive, createdAt) VALUES(?,?,?,?,?,?)`,
    ["key-a-uuid", keyA, "Key A", "machine-a", 1, "2026-01-01T00:00:00Z"]);
  db.run(`INSERT INTO apiKeys(id, key, name, machineId, isActive, createdAt) VALUES(?,?,?,?,?,?)`,
    ["key-b-uuid", keyB, "Key B", "machine-b", 1, "2026-01-01T00:00:00Z"]);

  const today = new Date();
  const todayKey = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);
  const yesterdayKey = `${yesterday.getFullYear()}-${String(yesterday.getMonth() + 1).padStart(2, "0")}-${String(yesterday.getDate()).padStart(2, "0")}`;

  db.run(`INSERT INTO usageDaily(dateKey, data) VALUES(?,?)`, [todayKey, JSON.stringify({
    requests: 3, promptTokens: 150, completionTokens: 225, cost: 0.015,
    byProvider: { openai: { requests: 2, promptTokens: 100, completionTokens: 150, cost: 0.01 }, anthropic: { requests: 1, promptTokens: 50, completionTokens: 75, cost: 0.005 } },
    byApiKey: {
      [keyA + "|gpt-4|openai"]: { requests: 2, promptTokens: 100, completionTokens: 150, cost: 0.01, rawModel: "gpt-4", provider: "openai", apiKey: keyA },
      [keyB + "|claude-3|anthropic"]: { requests: 1, promptTokens: 50, completionTokens: 75, cost: 0.005, rawModel: "claude-3", provider: "anthropic", apiKey: keyB },
    },
  })]);

  db.run(`INSERT INTO usageDaily(dateKey, data) VALUES(?,?)`, [yesterdayKey, JSON.stringify({
    requests: 2, promptTokens: 100, completionTokens: 200, cost: 0.01,
    byProvider: { openai: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005 }, anthropic: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005 } },
    byApiKey: {
      [keyA + "|gpt-4|openai"]: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005, rawModel: "gpt-4", provider: "openai", apiKey: keyA },
      [keyB + "|claude-3|anthropic"]: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005, rawModel: "claude-3", provider: "anthropic", apiKey: keyB },
    },
  })]);

  const now = new Date().toISOString();
  const hourAgo = new Date(Date.now() - 3600000).toISOString();
  db.run(`INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
    [now, "openai", "gpt-4", "conn-1", keyA, "/v1/chat", 50, 75, 0.005, "ok", JSON.stringify({ prompt_tokens: 50, completion_tokens: 75 })]);
  db.run(`INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
    [hourAgo, "anthropic", "claude-3", "conn-2", keyB, "/v1/messages", 50, 75, 0.005, "ok", JSON.stringify({ prompt_tokens: 50, completion_tokens: 75 })]);

  return { keyA, keyB };
}

function makeRequest(url) {
  return new Request(url, { headers: { "Content-Type": "application/json" } });
}

describe("GET /api/usage/per-key/[keyId]", () => {
  it("returns 200 with valid keyId and default period", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA } = seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest(`http://localhost/api/usage/per-key/${"key-a-uuid"}?period=7d`), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data).toHaveProperty("keyId", "key-a-uuid");
    expect(data).toHaveProperty("keyName", "Key A");
    expect(data.keyMasked).toBe(keyA.slice(0, 8) + "..." + keyA.slice(-4));
    expect(data.period).toBe("7d");
    expect(data).toHaveProperty("stats");
    expect(data).toHaveProperty("byModel");
    expect(data).toHaveProperty("chartData");
    expect(data).toHaveProperty("history");
  });

  it("returns 200 with explicit period=24h", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid?period=24h"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data.period).toBe("24h");
  });

  it("returns 404 for nonexistent keyId", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    await getAdapter();
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/nonexistent"), { params: Promise.resolve({ keyId: "nonexistent" }) });
    const data = await res.json();

    expect(res.status).toBe(404);
    expect(data.error).toBe("API key not found");
  });

  it("returns 400 for invalid period", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid?period=invalid"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(400);
    expect(data.error).toBe("Invalid period");
  });

  it("keyMasked format is correct", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data.keyMasked).toBe("sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaAA".slice(0, 8) + "..." + "sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaAA".slice(-4));
  });

  it("stats has correct structure", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data.stats).toHaveProperty("totalRequests");
    expect(data.stats).toHaveProperty("totalPromptTokens");
    expect(data.stats).toHaveProperty("totalCompletionTokens");
    expect(data.stats).toHaveProperty("totalCost");
    expect(typeof data.stats.totalRequests).toBe("number");
    expect(typeof data.stats.totalPromptTokens).toBe("number");
  });

  it("byModel entries have correct structure", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    if (data.byModel.length > 0) {
      const entry = data.byModel[0];
      expect(entry).toHaveProperty("name");
      expect(entry).toHaveProperty("provider");
      expect(entry).toHaveProperty("requests");
      expect(entry).toHaveProperty("promptTokens");
      expect(entry).toHaveProperty("completionTokens");
      expect(entry).toHaveProperty("cost");
      expect(entry).toHaveProperty("lastUsed");
    }
  });

  it("history entries have correct structure", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    if (data.history.length > 0) {
      const entry = data.history[0];
      expect(entry).toHaveProperty("timestamp");
      expect(entry).toHaveProperty("model");
      expect(entry).toHaveProperty("provider");
      expect(entry).toHaveProperty("cost");
      expect(entry).toHaveProperty("tokens");
    }
  });
});

describe("GET /api/usage/per-key/[keyId]/chart", () => {
  it("returns 200 with chart data for valid key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA } = seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/chart/route.js");

    const res = await GET(makeRequest(`http://localhost/api/usage/per-key/${"key-a-uuid"}/chart?period=7d`), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data).toHaveProperty("keyId", "key-a-uuid");
    expect(data).toHaveProperty("period", "7d");
    expect(data).toHaveProperty("chartData");
    expect(Array.isArray(data.chartData)).toBe(true);
    expect(data.chartData.length).toBe(7);
    if (data.chartData.length > 0) {
      expect(data.chartData[0]).toHaveProperty("label");
      expect(data.chartData[0]).toHaveProperty("tokens");
      expect(data.chartData[0]).toHaveProperty("cost");
    }
  });

  it("returns 400 for invalid period", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    await getAdapter();
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/chart/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid/chart?period=invalid"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(400);
    expect(data.error).toBe("Invalid period");
  });

  it("returns 404 for nonexistent key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    await getAdapter();
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/chart/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/nonexistent/chart"), { params: Promise.resolve({ keyId: "nonexistent" }) });
    const data = await res.json();

    expect(res.status).toBe(404);
    expect(data.error).toBe("API key not found");
  });

  it("defaults period to 7d", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/chart/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid/chart"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data.period).toBe("7d");
  });
});

describe("GET /api/usage/per-key/[keyId]/history", () => {
  it("returns 200 with history for valid key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA } = seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/history/route.js");

    const res = await GET(makeRequest(`http://localhost/api/usage/per-key/${"key-a-uuid"}/history?limit=50&offset=0`), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data).toHaveProperty("keyId", "key-a-uuid");
    expect(data).toHaveProperty("history");
    expect(Array.isArray(data.history)).toBe(true);
    expect(data).toHaveProperty("limit", 50);
    expect(data).toHaveProperty("offset", 0);
  });

  it("returns 404 for nonexistent key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    await getAdapter();
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/history/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/nonexistent/history"), { params: Promise.resolve({ keyId: "nonexistent" }) });
    const data = await res.json();

    expect(res.status).toBe(404);
    expect(data.error).toBe("API key not found");
  });

  it("respects limit parameter (capped at 200)", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/history/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid/history?limit=200"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    expect(data.limit).toBe(200);
  });

  it("history entries have correct structure", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { GET } = await import("@/app/api/usage/per-key/[keyId]/history/route.js");

    const res = await GET(makeRequest("http://localhost/api/usage/per-key/key-a-uuid/history"), { params: Promise.resolve({ keyId: "key-a-uuid" }) });
    const data = await res.json();

    expect(res.status).toBe(200);
    if (data.history.length > 0) {
      const entry = data.history[0];
      expect(entry).toHaveProperty("timestamp");
      expect(entry).toHaveProperty("model");
      expect(entry).toHaveProperty("provider");
      expect(entry).toHaveProperty("cost");
      expect(entry).toHaveProperty("tokens");
    }
  });
});
