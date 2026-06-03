import { describe, it, expect, beforeAll, afterAll } from "vitest";
const skip = !process.env.POSTGRES_URL;
describe.skipIf(skip)("PG E2E: Transaction isolation", () => {
  let db;
  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });
  afterAll(async () => { if (db) await db.close(); });
  beforeAll(async () => {
    await db.exec("CREATE TABLE IF NOT EXISTS pg_tx_test (id INT PRIMARY KEY, val TEXT)");
    await db.exec("DELETE FROM pg_tx_test");
  });

  it("transaction commits on success", async () => {
    await db.run("INSERT INTO pg_tx_test (id,val) VALUES ($1,$2)",[1,"inserted"]);
    await db.transaction(async (txn) => {
      await txn.run("UPDATE pg_tx_test SET val=$1 WHERE id=$2",["updated",1]);
      const r = await txn.get("SELECT * FROM pg_tx_test WHERE id=$1",[1]);
      expect(r.val).toBe("updated");
    });
    const r = await db.get("SELECT * FROM pg_tx_test WHERE id=$1",[1]);
    expect(r.val).toBe("updated");
  });

  it("transaction rolls back on error", async () => {
    await db.run("DELETE FROM pg_tx_test");
    await db.run("INSERT INTO pg_tx_test (id,val) VALUES ($1,$2)",[2,"before-rollback"]);
    try {
      await db.transaction(async (txn) => {
        await txn.run("UPDATE pg_tx_test SET val=$1 WHERE id=$2",["rolled-back",2]);
        throw new Error("intentional");
      });
    } catch (e) { /* expected */ }
    const r = await db.get("SELECT * FROM pg_tx_test WHERE id=$1",[2]);
    expect(r.val).toBe("before-rollback");
  });

  it("nested transactions use SAVEPOINT", async () => {
    await db.run("DELETE FROM pg_tx_test");
    await db.transaction(async (outer) => {
      await outer.run("INSERT INTO pg_tx_test (id,val) VALUES ($1,$2)",[3,"outer"]);
      await outer.transaction(async (inner) => {
        await inner.run("UPDATE pg_tx_test SET val=$1 WHERE id=$2",["inner-updated",3]);
        const r = await inner.get("SELECT * FROM pg_tx_test WHERE id=$1",[3]);
        expect(r.val).toBe("inner-updated");
      });
      const r = await outer.get("SELECT * FROM pg_tx_test WHERE id=$1",[3]);
      expect(r.val).toBe("inner-updated");
    });
    const final = await db.get("SELECT * FROM pg_tx_test WHERE id=$1",[3]);
    expect(final.val).toBe("inner-updated");
  });

  it("transaction returns value from fn", async () => {
    const result = await db.transaction(async (txn) => {
      await txn.run("INSERT INTO pg_tx_test (id,val) VALUES ($1,$2)",[4,"tx-return"]);
      return "success-value";
    });
    expect(result).toBe("success-value");
  });
});
describe.skipIf(!skip)("skipped", () => { it("skipped", () => expect(true).toBe(true)); });
