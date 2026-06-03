import { getAdapter } from "../driver.js";

export async function getMeta(key, fallback = null) {
  const db = await getAdapter();
  const row = db.get(`SELECT value FROM _meta WHERE key = ?`, [key]);
  return row ? row.value : fallback;
}

export async function setMeta(key, value) {
  const db = await getAdapter();
  db.run(`INSERT INTO _meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, [key, String(value)]);
}

// Atomically increment a _meta counter by `delta` and return the new value.
// Single statement — no read-then-write race. Works on PostgreSQL and
// SQLite (INSERT ... RETURNING is supported by all bundled drivers).
export async function incrementMeta(key, delta = 1) {
  const db = await getAdapter();
  const row = db.get(
    `INSERT INTO _meta(key, value) VALUES(?, CAST(? AS TEXT))
     ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(value AS INTEGER) + ? AS TEXT)
     RETURNING value`,
    [key, String(delta), delta]
  );
  return row ? parseInt(row.value, 10) : delta;
}

// Sync versions for use during migration and inside db.transaction() callbacks
// (some adapters — better-sqlite3, bun:sqlite, sql.js — cannot await inside
// transaction callbacks). Atomic increment, same portable SQL as incrementMeta.
export function getMetaSync(adapter, key, fallback = null) {
  const row = adapter.get(`SELECT value FROM _meta WHERE key = ?`, [key]);
  return row ? row.value : fallback;
}

export function setMetaSync(adapter, key, value) {
  adapter.run(`INSERT INTO _meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, [key, String(value)]);
}

export function incrementMetaSync(adapter, key, delta = 1) {
  const row = adapter.get(
    `INSERT INTO _meta(key, value) VALUES(?, CAST(? AS TEXT))
     ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(value AS INTEGER) + ? AS TEXT)
     RETURNING value`,
    [key, String(delta), delta]
  );
  return row ? parseInt(row.value, 10) : delta;
}
