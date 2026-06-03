import { getAdapter } from "../driver.js";
import { parseJson, stringifyJson } from "./jsonCol.js";

export function makeKv(scope) {
  const isPg = (db) => db.driver === "postgres";

  return {
    async get(key, fallback = null) {
      const db = await getAdapter();
      const row = await db.get(`SELECT value FROM kv WHERE scope = ? AND key = ?`, [scope, key]);
      if (!row) return fallback;
      const val = isPg(db) ? row.value : parseJson(row.value, fallback);
      return val ?? fallback;
    },
    async getAll() {
      const db = await getAdapter();
      const rows = await db.all(`SELECT key, value FROM kv WHERE scope = ?`, [scope]);
      const out = {};
      for (const r of rows) out[r.key] = isPg(db) ? (r.value ?? null) : parseJson(r.value);
      return out;
    },
    async set(key, value) {
      const db = await getAdapter();
      await db.run(`INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
        [scope, key, isPg(db) ? value : stringifyJson(value)]);
    },
    async setMany(obj) {
      const db = await getAdapter();
      await db.transaction(async (txn) => {
        const pg = isPg(db);
        for (const [k, v] of Object.entries(obj)) {
          await txn.run(`INSERT INTO kv(scope, key, value) VALUES(?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
            [scope, k, pg ? v : stringifyJson(v)]);
        }
      });
    },
    async remove(key) {
      const db = await getAdapter();
      await db.run(`DELETE FROM kv WHERE scope = ? AND key = ?`, [scope, key]);
    },
    async clear() {
      const db = await getAdapter();
      await db.run(`DELETE FROM kv WHERE scope = ?`, [scope]);
    },
  };
}
