import { describe, it, expect, beforeAll, afterAll } from "vitest";

const POSTGRES_URL = process.env.POSTGRES_URL;
const skip = !POSTGRES_URL;

describe.skipIf(skip)("PostgreSQL adapter interface conformance", () => {
  let db;

  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });

  afterAll(async () => {
    if (db) await db.close();
  });

  it("selects postgres driver when POSTGRES_URL is set", () => {
    expect(db.driver).toBe("postgres");
  });

  it("run() returns numeric changes and lastInsertRowid when RETURNING is used", async () => {
    const r = await db.run(
      "INSERT INTO settings (id, data) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data RETURNING id",
      [1, { test: true }]
    );
    expect(typeof r.changes).toBe("number");
    expect(r.changes).toBeGreaterThanOrEqual(0);
    // lastInsertRowid only populated when SQL has RETURNING <id>
    expect(typeof r.lastInsertRowid).toBe("number");
  });

  it("get() returns JSONB as JS object, not string", async () => {
    const row = await db.get("SELECT data FROM settings WHERE id = $1", [1]);
    expect(typeof row.data).toBe("object");
    expect(row.data.test).toBe(true);
  });

  it("all() returns an array", async () => {
    const rows = await db.all("SELECT * FROM settings");
    expect(Array.isArray(rows)).toBe(true);
  });

  it("exec() handles multi-statement DDL", async () => {
    await db.exec(`
      CREATE TABLE IF NOT EXISTS pg_adapter_test (id INT, name TEXT);
      CREATE INDEX IF NOT EXISTS idx_pat_name ON pg_adapter_test(name);
    `);
    const r = await db.get("SELECT to_regclass('pg_adapter_test') AS exists");
    expect(r.exists).toBeTruthy();
  });

  it("transaction() commits on success", async () => {
    await db.exec("CREATE TABLE IF NOT EXISTS pg_tx_test (id INT PRIMARY KEY, val TEXT)");
    await db.run("DELETE FROM pg_tx_test");
    await db.transaction(async (txn) => {
      await txn.run("INSERT INTO pg_tx_test (id, val) VALUES ($1, $2)", [1, "committed"]);
    });
    const row = await db.get("SELECT * FROM pg_tx_test WHERE id = 1");
    expect(row.val).toBe("committed");
  });

  it("transaction() rolls back on error", async () => {
    await db.run("DELETE FROM pg_tx_test");
    await expect(
      db.transaction(async (txn) => {
        await txn.run("INSERT INTO pg_tx_test (id, val) VALUES ($1, $2)", [2, "rolled-back"]);
        throw new Error("intentional");
      })
    ).rejects.toThrow("intentional");
    const row = await db.get("SELECT * FROM pg_tx_test WHERE id = 2");
    expect(row).toBeUndefined();
  });

  it("prepare() returns reusable { run, get, all } object", async () => {
    const stmt = await db.prepare("SELECT * FROM pg_tx_test WHERE id = $1");
    expect(typeof stmt.run).toBe("function");
    expect(typeof stmt.get).toBe("function");
    expect(typeof stmt.all).toBe("function");
  });

  it("checkpoint() is a no-op for PG (returns undefined)", () => {
    expect(db.checkpoint()).toBeUndefined();
  });

  it("placeholder translation: ? → $1, $2, ...", async () => {
    await db.run(
      "INSERT INTO pg_tx_test (id, val) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET val = EXCLUDED.val",
      [3, "qmark"]
    );
    const row = await db.get("SELECT * FROM pg_tx_test WHERE id = 3");
    expect(row.val).toBe("qmark");
  });

  it("BOOLEAN columns return JS booleans", async () => {
    await db.run(
      "INSERT INTO apiKeys (id, key, isActive, createdAt) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET isActive = EXCLUDED.isActive",
      ["pg-test-key", "pg-test-key-value", false, new Date().toISOString()]
    );
    const row = await db.get("SELECT isActive FROM apiKeys WHERE id = $1", ["pg-test-key"]);
    expect(row).toBeDefined();
    expect(typeof row.isactive).toBe("boolean");
    expect(row.isactive).toBe(false);
  });

  it("all 11 base tables exist after driver init", async () => {
    const expected = [
      "_meta", "settings", "providerConnections", "providerNodes", "proxyPools",
      "apiKeys", "combos", "kv", "usageHistory", "usageDaily", "requestDetails"
    ];
    for (const t of expected) {
      const r = await db.get("SELECT to_regclass($1) AS exists", [t]);
      expect(r.exists, "table " + t + " should exist").toBeTruthy();
    }
  });

  it("all 5 rollup tables exist", async () => {
    const rollups = [
      "usageDailyByProvider", "usageDailyByModel", "usageDailyByApiKey",
      "usageDailyByAccount", "usageDailyByEndpoint"
    ];
    for (const t of rollups) {
      const r = await db.get("SELECT to_regclass($1) AS exists", [t]);
      expect(r.exists, "rollup " + t + " should exist").toBeTruthy();
    }
  });
});

describe.skipIf(!skip)("PostgreSQL adapter (skipped — POSTGRES_URL not set)", () => {
  it("skips when POSTGRES_URL is not configured", () => {
    expect(true).toBe(true);
  });
});
