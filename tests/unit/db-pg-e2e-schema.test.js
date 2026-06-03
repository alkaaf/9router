import { describe, it, expect, beforeAll, afterAll } from "vitest";
const skip = !process.env.POSTGRES_URL;
describe.skipIf(skip)("PG E2E: Schema + migration", () => {
  let db;
  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });
  afterAll(async () => { if (db) await db.close(); });

  it("all 16 tables (11 base + 5 rollup) exist in PG", async () => {
    const tables = ["_meta","settings","providerConnections","providerNodes","proxyPools","apiKeys","combos","kv","usageHistory","usageDaily","requestDetails","usageDailyByProvider","usageDailyByModel","usageDailyByApiKey","usageDailyByAccount","usageDailyByEndpoint"];
    for (const t of tables) {
      const r = await db.get("SELECT to_regclass($1) AS exists",[t]);
      expect(r.exists, `table ${t} should exist`).toBeTruthy();
    }
  });

  it("all expected columns exist in usageHistory", async () => {
    const expected = ["id","timestamp","provider","model","connectionId","apiKey","endpoint","promptTokens","completionTokens","cost","status","tokens","meta"];
    const rows = await db.all("SELECT * FROM usageHistory LIMIT 0");
    const actual = rows.length === 0 ? [] : Object.keys(rows[0]);
    for (const col of expected) {
      const found = actual.some(k => k.toLowerCase() === col.toLowerCase());
      expect(found, `column ${col} should exist in usageHistory`).toBe(true);
    }
  });

  it("schema.postgres.js DDL is idempotent (re-apply doesn't error)", async () => {
    const { getPostgresSchema } = await import("../../src/lib/db/schema.postgres.js");
    const schema = getPostgresSchema();
    // Run all CREATE TABLE IF NOT EXISTS statements
    await db.exec(schema);
    // Should not error — IF NOT EXISTS makes it idempotent
    expect(true).toBe(true);
  });

  it("inserts work on re-initialized schema", async () => {
    const r = await db.run(`INSERT INTO settings (id,data) VALUES (1,$1) ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`,[{postinit:"test"}]);
    expect(typeof r.changes).toBe("number");
    const row = await db.get("SELECT * FROM settings WHERE id=1");
    expect(row.data.postinit).toBe("test");
  });

  it("numeric types (NUMERIC(12,6)) handle decimal correctly", async () => {
    const today = new Date().toISOString().split("T")[0];
    await db.run(`INSERT INTO usageDailyByProvider (date,provider,requestCount,inputTokens,outputTokens,totalTokens,cost) VALUES ($1,$2,$3,$4,$5,$6,$7)`,[today,"num-test",1,10,5,15,0.001234]);
    const row = await db.get("SELECT cost FROM usageDailyByProvider WHERE provider=$1",["num-test"]);
    expect(typeof row.cost).toBe("number");
    expect(row.cost).toBeCloseTo(0.001234, 6);
  });
});
describe.skipIf(!skip)("skipped", () => { it("skipped", () => expect(true).toBe(true)); });
