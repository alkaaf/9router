import { describe, it, expect, beforeAll, afterAll } from "vitest";
const skip = !process.env.POSTGRES_URL;
describe.skipIf(skip)("PG E2E: kvStore JSONB round-trip", () => {
  let db;
  beforeAll(async () => {
    const { getAdapter } = await import("../../src/lib/db/driver.js");
    db = await getAdapter();
  });
  afterAll(async () => { if (db) await db.close(); });
  beforeAll(async () => { await db.exec("DELETE FROM kv WHERE scope='pg-e2e-kv'"); });

  it("stores and retrieves complex nested objects as JSONB", async () => {
    const obj = { nested: {a:1,b:[1,2,3]}, bool:true, nullVal:null, emptyArr:[], emptyObj:{} };
    await db.run(`INSERT INTO kv (scope,key,value) VALUES ($1,$2,$3) ON CONFLICT (scope,key) DO UPDATE SET value=EXCLUDED.value`,["pg-e2e-kv","complex",JSON.stringify(obj)]);
    const row = await db.get("SELECT * FROM kv WHERE scope=$1 AND key=$2",["pg-e2e-kv","complex"]);
    expect(row).toBeDefined();
    expect(typeof row.value).toBe("object","PG should return JSONB as parsed object, got: " + typeof row.value);
    expect(row.value.nested.a).toBe(1);
    expect(row.value.bool).toBe(true);
    expect(row.value.nullVal).toBeNull();
    expect(row.value.emptyArr).toEqual([]);
    expect(row.value.emptyObj).toEqual({});
  });

  it("stores and retrieves unicode and emoji", async () => {
    const obj = { msg: "Hello \ud83e\udd80 \u4f60\u597d \u03b1\u03b2\u03b3\u03b4" };
    await db.run(`INSERT INTO kv (scope,key,value) VALUES ($1,$2,$3) ON CONFLICT (scope,key) DO UPDATE SET value=EXCLUDED.value`,["pg-e2e-kv","unicode",JSON.stringify(obj)]);
    const row = await db.get("SELECT * FROM kv WHERE scope=$1 AND key=$2",["pg-e2e-kv","unicode"]);
    expect(row.value.msg).toBe("Hello \ud83e\udd80 \u4f60\u597d \u03b1\u03b2\u03b3\u03b4");
  });

  it("stores and retrieves falsy values (0, false, empty string)", async () => {
    const obj = { zero:0, emptyStr:"", falseBool:false };
    await db.run(`INSERT INTO kv (scope,key,value) VALUES ($1,$2,$3) ON CONFLICT (scope,key) DO UPDATE SET value=EXCLUDED.value`,["pg-e2e-kv","falsy",JSON.stringify(obj)]);
    const row = await db.get("SELECT * FROM kv WHERE scope=$1 AND key=$2",["pg-e2e-kv","falsy"]);
    expect(row.value.zero).toBe(0);
    expect(row.value.emptyStr).toBe("");
    expect(row.value.falseBool).toBe(false);
  });

  it("composite PK (scope, key) ON CONFLICT works correctly", async () => {
    await db.run(`INSERT INTO kv (scope,key,value) VALUES ($1,$2,$3) ON CONFLICT (scope,key) DO UPDATE SET value=EXCLUDED.value`,["pg-e2e-kv","dup",JSON.stringify("first")]);
    await db.run(`INSERT INTO kv (scope,key,value) VALUES ($1,$2,$3) ON CONFLICT (scope,key) DO UPDATE SET value=EXCLUDED.value`,["pg-e2e-kv","dup",JSON.stringify("second")]);
    const row = await db.get("SELECT * FROM kv WHERE scope=$1 AND key=$2",["pg-e2e-kv","dup"]);
    expect(row.value).toBe("second");
  });
});
describe.skipIf(!skip)("skipped", () => { it("skipped", () => expect(true).toBe(true)); });
