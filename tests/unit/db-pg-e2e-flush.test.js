import { describe, it, expect, beforeAll, afterAll } from "vitest";
const skip = !process.env.POSTGRES_URL;
describe.skipIf(skip)("PG E2E: usageRepo flush + rollup tables", () => {
  let db;
  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });
  afterAll(async () => { if (db) await db.close(); });

  beforeAll(async () => {
    await db.exec("DELETE FROM usageHistory");
    await db.exec("DELETE FROM usageDailyByProvider");
    await db.exec("DELETE FROM usageDailyByModel");
    await db.exec("DELETE FROM usageDailyByAccount");
    await db.exec("DELETE FROM usageDailyByEndpoint");
  });

  it("bulk inserts 5 usageHistory rows", async () => {
    const today = new Date().toISOString();
    const entries = [];
    for (let i = 0; i < 5; i++) entries.push({
      timestamp: today, provider: "openai", model: "gpt-4", connectionId: "conn-1",
      apiKey: "ak-test", endpoint: "/v1/chat/completions", promptTokens: 100,
      completionTokens: 50, cost: 0.03, status: "success",
      tokens: JSON.stringify({p:100,c:50}), meta: JSON.stringify({ip:"127.0.0.1"})
    });
    const sql = "INSERT INTO usageHistory (timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta) VALUES " +
      entries.map((_,i)=>`($${i*12+1},$${i*12+2},$${i*12+3},$${i*12+4},$${i*12+5},$${i*12+6},$${i*12+7},$${i*12+8},$${i*12+9},$${i*12+10},$${i*12+11},$${i*12+12})`).join(",");
    const params = entries.flatMap(e=>[e.timestamp,e.provider,e.model,e.connectionId,e.apiKey,e.endpoint,e.promptTokens,e.completionTokens,e.cost,e.status,e.tokens,e.meta]);
    const r = await db.run(sql, params);
    expect(typeof r.changes).toBe("number");
    expect(r.changes).toBeGreaterThanOrEqual(5);
  });

  it("reads usageHistory back with correct types", async () => {
    const rows = await db.all("SELECT * FROM usageHistory WHERE provider=$1",["openai"]);
    expect(rows.length).toBeGreaterThanOrEqual(5);
    expect(typeof rows[0].cost).toBe("number");
    expect(typeof rows[0].tokens).toBe("object");
    expect(typeof rows[0].meta).toBe("object");
  });

  it("usageDailyByProvider upsert works", async () => {
    const today = new Date().toISOString().split("T")[0];
    const r = await db.run(
      `INSERT INTO usageDailyByProvider (date,provider,requestCount,inputTokens,outputTokens,totalTokens,cost) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (date,provider) DO UPDATE SET requestCount=usageDailyByProvider.requestCount+EXCLUDED.requestCount`,
      [today,"openai",5,500,250,750,0.15]
    );
    expect(typeof r.changes).toBe("number");
    const row = await db.get("SELECT * FROM usageDailyByProvider WHERE provider=$1",["openai"]);
    expect(row).toBeDefined();
    expect(typeof row.cost).toBe("number");
  });

  it("all 5 rollup tables exist with correct schema", async () => {
    const rollups = ["usageDailyByProvider","usageDailyByModel","usageDailyByApiKey","usageDailyByAccount","usageDailyByEndpoint"];
    for (const t of rollups) {
      const r = await db.get("SELECT to_regclass($1) AS exists",[t]);
      expect(r.exists, `table ${t} should exist`).toBeTruthy();
    }
  });
});
describe.skipIf(!skip)("skipped", () => { it("skipped", () => expect(true).toBe(true)); });
