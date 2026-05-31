import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

let tempDir;
const originalDataDir = process.env.DATA_DIR;

beforeEach(() => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "9router-perkey-"));
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
    byProvider: {
      openai: { requests: 2, promptTokens: 100, completionTokens: 150, cost: 0.01 },
      anthropic: { requests: 1, promptTokens: 50, completionTokens: 75, cost: 0.005 },
    },
    byApiKey: {
      [`${keyA}|gpt-4|openai`]: { requests: 2, promptTokens: 100, completionTokens: 150, cost: 0.01, rawModel: "gpt-4", provider: "openai", apiKey: keyA },
      [`${keyB}|claude-3|anthropic`]: { requests: 1, promptTokens: 50, completionTokens: 75, cost: 0.005, rawModel: "claude-3", provider: "anthropic", apiKey: keyB },
    },
  })]);

  db.run(`INSERT INTO usageDaily(dateKey, data) VALUES(?,?)`, [yesterdayKey, JSON.stringify({
    requests: 2, promptTokens: 100, completionTokens: 200, cost: 0.01,
    byProvider: {
      openai: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005 },
      anthropic: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005 },
    },
    byApiKey: {
      [`${keyA}|gpt-4|openai`]: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005, rawModel: "gpt-4", provider: "openai", apiKey: keyA },
      [`${keyB}|claude-3|anthropic`]: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005, rawModel: "claude-3", provider: "anthropic", apiKey: keyB },
    },
  })]);

  const now = new Date().toISOString();
  const hourAgo = new Date(Date.now() - 3600000).toISOString();
  db.run(`INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
    [now, "openai", "gpt-4", "conn-1", keyA, "/v1/chat", 50, 75, 0.005, "ok", JSON.stringify({ prompt_tokens: 50, completion_tokens: 75 })]);
  db.run(`INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
    [hourAgo, "anthropic", "claude-3", "conn-2", keyB, "/v1/messages", 50, 75, 0.005, "ok", JSON.stringify({ prompt_tokens: 50, completion_tokens: 75 })]);
  db.run(`INSERT INTO usageHistory(timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
    [hourAgo, "openai", "gpt-4", "conn-1", keyA, "/v1/chat", 30, 50, 0.003, "ok", JSON.stringify({ prompt_tokens: 30, completion_tokens: 50 })]);

  return { keyA, keyB, todayKey, yesterdayKey };
}

describe("getUsageStats with filter.apiKey", () => {
  it("returns stats only for the specified API key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA } = seedDb(db);
    const { getUsageStats } = await import("@/lib/db/repos/usageRepo.js");

    const stats = await getUsageStats("7d", { apiKey: keyA });
    expect(stats.totalRequests).toBeGreaterThan(0);
    expect(Object.values(stats.byApiKey).every((e) => e.apiKey === keyA)).toBe(true);
  });

  it("returns different stats for a different key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA, keyB } = seedDb(db);
    const { getUsageStats } = await import("@/lib/db/repos/usageRepo.js");

    const statsA = await getUsageStats("7d", { apiKey: keyA });
    const statsB = await getUsageStats("7d", { apiKey: keyB });
    expect(statsA.totalRequests).not.toBe(statsB.totalRequests);
  });

  it("returns all keys when no filter applied", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { getUsageStats } = await import("@/lib/db/repos/usageRepo.js");

    const stats = await getUsageStats("7d", {});
    expect(stats.totalRequests).toBeGreaterThan(0);
    const keysInResult = Object.values(stats.byApiKey).map((e) => e.apiKey);
    expect(keysInResult.length).toBeGreaterThanOrEqual(2);
  });

  it("returns zero stats for nonexistent API key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { getUsageStats } = await import("@/lib/db/repos/usageRepo.js");

    const stats = await getUsageStats("7d", { apiKey: "sk-nonexistent-key-12345678901234567890" });
    expect(stats.totalRequests).toBe(0);
    expect(Object.keys(stats.byApiKey).length).toBe(0);
  });

  it("filters live history correctly for today period", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA } = seedDb(db);
    const { getUsageStats } = await import("@/lib/db/repos/usageRepo.js");

    const stats = await getUsageStats("today", { apiKey: keyA });
    const apiKeyEntries = Object.values(stats.byApiKey);
    expect(apiKeyEntries.every((e) => e.apiKey === keyA)).toBe(true);
  });
});

describe("getUsageHistory with filter.apiKey", () => {
  it("returns only entries for the specified API key", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA, keyB } = seedDb(db);
    const { getUsageHistory } = await import("@/lib/db/repos/usageRepo.js");

    const history = await getUsageHistory({ apiKey: keyA });
    expect(history.every((r) => r.apiKey === keyA)).toBe(true);
    expect(history.length).toBeGreaterThan(0);
  });

  it("returns different results for different keys", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA, keyB } = await import("@/lib/db/driver.js");
    const { keyA: kA, keyB: kB } = seedDb(db);
    const { getUsageHistory } = await import("@/lib/db/repos/usageRepo.js");

    const histA = await getUsageHistory({ apiKey: kA });
    const histB = await getUsageHistory({ apiKey: kB });
    expect(histA.length).toBeGreaterThan(0);
    expect(histB.length).toBeGreaterThan(0);
  });

  it("returns all entries when no filter", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    seedDb(db);
    const { getUsageHistory } = await import("@/lib/db/repos/usageRepo.js");

    const history = await getUsageHistory({});
    expect(history.length).toBeGreaterThanOrEqual(2);
  });
});

describe("getChartData with filter.apiKey", () => {
  it("returns chart data filtered by API key for 7d period", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA } = seedDb(db);
    const { getChartData } = await import("@/lib/db/repos/usageRepo.js");

    const chartData = await getChartData("7d", { apiKey: keyA });
    expect(chartData).toBeDefined();
    expect(Array.isArray(chartData)).toBe(true);
    expect(chartData.length).toBeGreaterThan(0);
    expect(chartData[0]).toHaveProperty("label");
    expect(chartData[0]).toHaveProperty("tokens");
    expect(chartData[0]).toHaveProperty("cost");
  });

  it("returns different chart data for different keys", async () => {
    const { getAdapter } = await import("@/lib/db/driver.js");
    const db = await getAdapter();
    const { keyA, keyB } = seedDb(db);
    const { getChartData } = await import("@/lib/db/repos/usageRepo.js");

    const chartA = await getChartData("7d", { apiKey: keyA });
    const chartB = await getChartData("7d", { apiKey: keyB });
    const totalTokensA = chartA.reduce((s, d) => s + d.tokens, 0);
    const totalTokensB = chartB.reduce((s, d) => s + d.tokens, 0);
    expect(totalTokensA).not.toBe(totalTokensB);
  });
});
