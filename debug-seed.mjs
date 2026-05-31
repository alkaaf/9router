import { getAdapter } from "@/lib/db/driver.js";
import fs from "node:fs";
import os from "node:os";

const tempDir = fs.mkdtempSync(os.tmpdir());
process.env.DATA_DIR = tempDir;
delete global._dbAdapter;
const db = await getAdapter();

const keyA = "sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaAA";
db.run(`INSERT INTO apiKeys(id, key, name, machineId, isActive, createdAt) VALUES(?,?,?,?,?,?)`,
  ["key-a-uuid", keyA, "Key A", "machine-a", 1, "2026-01-01T00:00:00Z"]);

const today = new Date();
const todayKey = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;
const yesterday = new Date(today);
yesterday.setDate(yesterday.getDate() - 1);
const yesterdayKey = `${yesterday.getFullYear()}-${String(yesterday.getMonth() + 1).padStart(2, "0")}-${String(yesterday.getDate()).padStart(2, "0")}`;

db.run(`INSERT INTO usageDaily(dateKey, data) VALUES(?,?)`, [todayKey, JSON.stringify({
  requests: 3, promptTokens: 150, completionTokens: 225, cost: 0.015,
  byApiKey: {
    [keyA + "|gpt-4|openai"]: { requests: 2, promptTokens: 100, completionTokens: 150, cost: 0.01, rawModel: "gpt-4", provider: "openai", apiKey: keyA },
  },
})]);

db.run(`INSERT INTO usageDaily(dateKey, data) VALUES(?,?)`, [yesterdayKey, JSON.stringify({
  requests: 2, promptTokens: 100, completionTokens: 200, cost: 0.01,
  byApiKey: {
    [keyA + "|gpt-4|openai"]: { requests: 1, promptTokens: 50, completionTokens: 100, cost: 0.005, rawModel: "gpt-4", provider: "openai", apiKey: keyA },
  },
})]);

const rows = db.all("SELECT dateKey, data FROM usageDaily");
for (const r of rows) {
  const d = JSON.parse(r.data);
  console.log(r.dateKey, "requests:", d.requests, "promptTokens:", d.promptTokens, "byApiKey keys:", Object.keys(d.byApiKey || {}));
}

const { getUsageStats } = await import("@/lib/db/repos/usageRepo.js");
const stats = await getUsageStats("7d", { apiKey: keyA });
console.log("\ngetUsageStats result:");
console.log("totalRequests:", stats.totalRequests);
console.log("totalPromptTokens:", stats.totalPromptTokens);
console.log("totalCompletionTokens:", stats.totalCompletionTokens);
console.log("totalCost:", stats.totalCost);
console.log("byApiKey entries:", Object.keys(stats.byApiKey));

try { global._dbAdapter?.instance?.close?.(); } catch {}
fs.rmSync(tempDir, { recursive: true, force: true });
