import { describe, it, expect, beforeAll, afterAll } from "vitest";
const skip = !process.env.POSTGRES_URL;
describe.skipIf(skip)("PG E2E: Boolean + NULL handling", () => {
  let db;
  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });
  afterAll(async () => { if (db) await db.close(); });

  describe("BOOLEAN columns", () => {
    beforeAll(async () => {
      await db.exec("DELETE FROM apiKeys WHERE id LIKE 'pg-bool-%'");
    });

    it("isActive=true stores and retrieves as boolean", async () => {
      await db.run(`INSERT INTO apiKeys (id,key,isActive,createdAt) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO UPDATE SET isActive=EXCLUDED.isActive`,["pg-bool-true","key-true",true,new Date().toISOString()]);
      const row = await db.get("SELECT isActive FROM apiKeys WHERE id=$1",["pg-bool-true"]);
      expect(typeof row.isActive).toBe("boolean","isActive type: " + typeof row.isActive + " value: " + row.isActive);
      expect(row.isActive).toBe(true);
    });

    it("isActive=false stores and retrieves as boolean", async () => {
      await db.run(`INSERT INTO apiKeys (id,key,isActive,createdAt) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO UPDATE SET isActive=EXCLUDED.isActive`,["pg-bool-false","key-false",false,new Date().toISOString()]);
      const row = await db.get("SELECT isActive FROM apiKeys WHERE id=$1",["pg-bool-false"]);
      expect(typeof row.isActive).toBe("boolean","isActive type: " + typeof row.isActive + " value: " + row.isActive);
      expect(row.isActive).toBe(false);
    });

    it("providerConnections isActive works", async () => {
      await db.run(`INSERT INTO providerConnections (id,provider,authType,isActive,createdAt,updatedAt,data) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO UPDATE SET isActive=EXCLUDED.isActive`,["pg-bool-pc","openai","api-key",false,new Date().toISOString(),new Date().toISOString(),JSON.stringify({})]);
      const row = await db.get("SELECT isActive FROM providerConnections WHERE id=$1",["pg-bool-pc"]);
      expect(typeof row.isActive).toBe("boolean");
      expect(row.isActive).toBe(false);
    });
  });

  describe("NULL handling", () => {
    beforeAll(async () => {
      await db.exec("DELETE FROM usageHistory WHERE apiKey LIKE 'pg-null-%'");
    });

    it("NULL connectionId stays NULL (not 0, not undefined)", async () => {
      const ts = new Date().toISOString();
      await db.run(`INSERT INTO usageHistory (timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,[ts,"null-test","model-x",null,"pg-null-1","/test",0,0,0,"success","{}","{}"]);
      const row = await db.get("SELECT connectionId FROM usageHistory WHERE apiKey=$1",["pg-null-1"]);
      expect(row.connectionId).toBeNull();
    });

    it("NULL in JSONB field (meta) stays null", async () => {
      const ts = new Date().toISOString();
      const meta = JSON.stringify({nullField: null, normalField: "value"});
      await db.run(`INSERT INTO usageHistory (timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,[ts,"null-jsonb","model-y","c-null","pg-null-2","/test",0,0,0,"success","{}","{\"nullField\":null}"]);
      const row = await db.get("SELECT meta FROM usageHistory WHERE apiKey=$1",["pg-null-2"]);
      expect(row.meta).toBeDefined();
      expect(row.meta.nullField).toBeNull();
    });
  });
});
describe.skipIf(!skip)("skipped", () => { it("skipped", () => expect(true).toBe(true)); });
