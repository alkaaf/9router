import { describe, it, expect, beforeAll, afterAll } from "vitest";
const skip = !process.env.POSTGRES_URL;
describe.skipIf(skip)("PG E2E: DATE + TIMESTAMPTZ handling", () => {
  let db;
  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });
  afterAll(async () => { if (db) await db.close(); });
  beforeAll(async () => {
    await db.exec("DELETE FROM usageHistory WHERE provider='date-test'");
    await db.exec("DELETE FROM usageDailyByProvider WHERE provider='date-test'");
  });

  it("TIMESTAMPTZ accepts ISO 8601 string and returns as string", async () => {
    const isoString = new Date().toISOString();
    await db.run(`INSERT INTO usageHistory (timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,[isoString,"date-test","gpt-4","c1","ak1","/v1/chat/completions",100,50,0.03,"success","{}","{}"]);
    const row = await db.get("SELECT timestamp FROM usageHistory WHERE provider=$1",["date-test"]);
    expect(row).toBeDefined();
    expect(typeof row.timestamp).toBe("string","timestamp should be string, got: " + typeof row.timestamp);
    expect(row.timestamp).toContain("T");
  });

  it("DATE accepts YYYY-MM-DD string", async () => {
    const today = new Date().toISOString().split("T")[0];
    const r = await db.run(`INSERT INTO usageDailyByProvider (date,provider,requestCount,inputTokens,outputTokens,totalTokens,cost) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (date,provider) DO UPDATE SET requestCount=usageDailyByProvider.requestCount+1`,[today,"date-test",1,10,5,15,0.01]);
    expect(typeof r.changes).toBe("number");
    const row = await db.get("SELECT date FROM usageDailyByProvider WHERE provider=$1",["date-test"]);
    expect(row).toBeDefined();
  });

  it("DATE range query works", async () => {
    const today = new Date().toISOString().split("T")[0];
    const yesterday = new Date(Date.now() - 86400000).toISOString().split("T")[0];
    await db.run(`INSERT INTO usageDailyByProvider (date,provider,requestCount,inputTokens,outputTokens,totalTokens,cost) VALUES ($1,$2,$3,$4,$5,$6,$7)`,[yesterday,"date-test",1,10,5,15,0.01]);
    const rows = await db.all("SELECT * FROM usageDailyByProvider WHERE provider=$1 AND date >= $2",["date-test",yesterday]);
    expect(rows.length).toBeGreaterThanOrEqual(1);
  });

  it("TIMESTAMPTZ comparison with date range works", async () => {
    const now = new Date();
    const isoNow = now.toISOString();
    const hourAgo = new Date(now - 3600000).toISOString();
    await db.run(`INSERT INTO usageHistory (timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,[isoNow,"date-test","gpt-4","c2","ak2","/v1/chat/completions",100,50,0.03,"success","{}","{}"]);
    const rows = await db.all("SELECT * FROM usageHistory WHERE provider=$1 AND timestamp >= $2",["date-test",hourAgo]);
    expect(rows.length).toBeGreaterThanOrEqual(1);
  });
});
describe.skipIf(!skip)("skipped", () => { it("skipped", () => expect(true).toBe(true)); });
