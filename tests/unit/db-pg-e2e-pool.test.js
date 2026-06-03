import { describe, it, expect, beforeAll, afterAll } from "vitest";
const skip = !process.env.POSTGRES_URL;
describe.skipIf(skip)("PG E2E: Pool + concurrency", () => {
  let db;
  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });
  afterAll(async () => { if (db) await db.close(); });
  beforeAll(async () => {
    await db.exec("DELETE FROM usageHistory WHERE apiKey LIKE 'pool-test-%'");
  });

  it("handles 10 parallel queries without hanging", async () => {
    const promises = [];
    for (let i = 0; i < 10; i++) {
      const idx = i;
      promises.push((async () => {
        const ts = new Date().toISOString();
        await db.run(`INSERT INTO usageHistory (timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,[ts,"pool-test","model","c"+idx,"pool-test-"+idx,"/test",10,5,0.001,"success","{}","{}"]);
        return await db.get("SELECT * FROM usageHistory WHERE apiKey=$1",["pool-test-"+idx]);
      })());
    }
    const results = await Promise.all(promises);
    expect(results.length).toBe(10);
    for (const r of results) {
      expect(r).toBeDefined();
    }
  });

  it("handles 5 parallel transactions", async () => {
    await db.exec("DELETE FROM pg_tx_test");
    const promises = [];
    for (let i = 0; i < 5; i++) {
      const idx = i;
      promises.push((async () => {
        return await db.transaction(async (txn) => {
          await txn.run("INSERT INTO pg_tx_test (id,val) VALUES ($1,$2)",[100+idx,"pool-tx-"+idx]);
          await new Promise(r => setTimeout(r, 50));
          const r = await txn.get("SELECT * FROM pg_tx_test WHERE id=$1",[100+idx]);
          return r.val;
        });
      })());
    }
    const results = await Promise.all(promises);
    expect(results.length).toBe(5);
    expect(results.every(v => v.startsWith("pool-tx-"))).toBe(true);
  });

  it("sequential queries work after many parallel ones", async () => {
    const r = await db.get("SELECT COUNT(*) as cnt FROM usageHistory WHERE provider='pool-test'");
    expect(typeof r.cnt).toBe("number");
    expect(r.cnt).toBeGreaterThanOrEqual(10);
  });
});
describe.skipIf(!skip)("skipped", () => { it("skipped", () => expect(true).toBe(true)); });
