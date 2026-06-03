// PostgreSQL adapter for 9Router — uses node-postgres (pg) connection pool.
//
// Mirrors the interface exposed by the SQLite adapters (see betterSqliteAdapter.js):
//   run / get / all / exec / transaction / prepare / checkpoint / close / raw
//
// Notes:
//   * SQLite-style "?" placeholders are translated to pg's "$1, $2, ..." form.
//   * JSONB columns: pg auto-serializes JS objects on write and auto-parses on
//     read, so we do NOT call JSON.stringify / JSON.parse here.
//   * BOOLEAN columns: pg maps JS booleans <-> BOOLEAN natively; pass true/false
//     directly. There is no "1/0" coercion at this layer — the schema and
//     repos are expected to use real boolean values.
//   * `rowCount` and `id` from RETURNING are returned as strings by pg; we
//     coerce to Number for parity with the SQLite adapters.
//   * `checkpoint()` is a no-op — WAL is a SQLite concept.

import pg from "pg";

const { Pool } = pg;

// Singleton pool — survive Next.js dev hot-reload by stashing on globalThis,
// same pattern used in the SQLite adapters and driver.js.
if (!global._pgPool) global._pgPool = { pool: null, prepared: new Map() };
const state = global._pgPool;

// Translate SQLite-style "?" placeholders to pg-style "$1, $2, ...".
// Counts sequential "?" only (does not attempt to skip over string literals —
// callers are expected to use parameter binding for literals, not inlined SQL).
function convertPlaceholders(sql) {
  let i = 0;
  return sql.replace(/\?/g, () => `$${++i}`);
}

// Build a per-transaction adapter that pins every call to a single checked-out
// client, so BEGIN/COMMIT/ROLLBACK all run on the same connection.
function createTransactionAdapter(client, parent) {
  return {
    driver: parent.driver,
    kind: parent.kind,

    async run(sql, params) {
      const r = await client.query(convertPlaceholders(sql), params);
      // lastInsertRowid is only available when the query has RETURNING <id>
      const id = r.rows[0]?.id;
      return { changes: Number(r.rowCount), lastInsertRowid: id !== undefined ? Number(id) : undefined };
    },

    async get(sql, params) {
      const r = await client.query(convertPlaceholders(sql), params);
      return r.rows[0];
    },

    async all(sql, params) {
      const r = await client.query(convertPlaceholders(sql), params);
      return r.rows;
    },

    async exec(sql, params) {
      const r = await client.query(convertPlaceholders(sql), params);
      return {
        lastInsertRowid: Number(r.rows[0]?.id ?? 0),
        changes: Number(r.rowCount),
      };
    },

    // Nested transactions degrade to savepoints — Postgres BEGIN inside a
    // transaction throws "already in transaction", so use SAVEPOINT instead.
    async transaction(fn) {
      const sp = `sp_${Math.random().toString(36).slice(2)}`;
      await client.query(`SAVEPOINT ${sp}`);
      try {
        const r = await fn(this);
        await client.query(`RELEASE SAVEPOINT ${sp}`);
        return r;
      } catch (e) {
        try {
          await client.query(`ROLLBACK TO SAVEPOINT ${sp}`);
          await client.query(`RELEASE SAVEPOINT ${sp}`);
        } catch {}
        throw e;
      }
    },

    prepare(sql) {
      // Bind to the txn client so prepared statements stay on the same
      // connection as BEGIN/COMMIT.
      return parent.prepare(sql, client);
    },

    checkpoint() { return; },

    async close() {
      // No-op for a transactional adapter; the client is released by the
      // outer transaction() finally block.
    },

    get raw() { return client; },
  };
}

export async function createPostgresAdapter(config = {}) {
  if (!state.pool) {
    const connectionString =
      config.connectionString ||
      process.env.POSTGRES_URL ||
      process.env.DATABASE_URL;

    if (!connectionString) {
      throw new Error(
        "[DB] PostgreSQL adapter requires POSTGRES_URL or DATABASE_URL env var (or config.connectionString)"
      );
    }

    state.pool = new Pool({
      connectionString,
      max: config.max ?? config.poolSize ?? parseInt(process.env.POSTGRES_POOL_SIZE || "20", 10),
      idleTimeoutMillis: config.idleTimeoutMillis ?? 30000,
      connectionTimeoutMillis: config.connectionTimeoutMillis ?? 10000,
      ssl:
        config.ssl ??
        (process.env.POSTGRES_SSL === "true" ? { rejectUnauthorized: false } : false),
    });

    // Surface pool-level errors (idle client failures etc.) to the log.
    state.pool.on("error", (err) => {
      console.error("[DB] pg pool error:", err.message);
    });
  }

  const pool = state.pool;

  // Cached prepared SQL. pg's Pool does not expose persistent prepared
  // statements, so we cache the converted SQL and re-issue pool.query() per
  // call. The cache avoids the placeholder-translation cost on hot paths.
  const prepared = state.prepared;

  function getPreparedEntry(sql, clientOverride) {
    let entry = prepared.get(sql);
    if (!entry) {
      entry = { sql: convertPlaceholders(sql) };
      prepared.set(sql, entry);
    }
    const c = clientOverride || pool;
    return {
      run: async (params = []) => {
        const r = await c.query(entry.sql, params);
        return { changes: Number(r.rowCount) };
      },
      get: async (params = []) => {
        const r = await c.query(entry.sql, params);
        return r.rows[0];
      },
      all: async (params = []) => {
        const r = await c.query(entry.sql, params);
        return r.rows;
      },
    };
  }

  const adapter = {
    driver: "postgres",
    kind: "postgres",

    async run(sql, params) {
      const r = await pool.query(convertPlaceholders(sql), params);
      // lastInsertRowid is only populated when SQL includes RETURNING <id>
      // pg lowercases column names, so result is { id: <value> }
      const id = r.rows[0]?.id;
      return { changes: Number(r.rowCount), lastInsertRowid: id !== undefined ? Number(id) : undefined };
    },

    async get(sql, params) {
      const r = await pool.query(convertPlaceholders(sql), params);
      return r.rows[0];
    },

    async all(sql, params) {
      const r = await pool.query(convertPlaceholders(sql), params);
      return r.rows;
    },

    async exec(sql, params) {
      // pg's query() only accepts a single statement; split on `;` boundaries
      // so DDL bundles (e.g. the full Postgres schema) can be passed in one
      // call, matching the SQLite adapters' multi-statement exec() contract.
      const statements = sql.split(/;\s*(?:\n|$)/).map((s) => s.trim()).filter(Boolean);
      let lastRow = null;
      for (const stmt of statements) {
        const r = await pool.query(convertPlaceholders(stmt), params);
        lastRow = r;
      }
      return {
        lastInsertRowid: Number(lastRow?.rows[0]?.id ?? 0),
        changes: Number(lastRow?.rowCount ?? 0),
      };
    },

    async transaction(fn) {
      const client = await pool.connect();
      try {
        await client.query("BEGIN");
        const txAdapter = createTransactionAdapter(client, adapter);
        const result = await fn(txAdapter);
        await client.query("COMMIT");
        return result;
      } catch (e) {
        try { await client.query("ROLLBACK"); } catch {}
        throw e;
      } finally {
        client.release();
      }
    },

    prepare(sql) {
      return getPreparedEntry(sql, null);
    },

    checkpoint() {
      // No-op: WAL is a SQLite concept. pg uses server-side WAL controlled by
      // the `wal_level` setting, not something we drive from the client.
      return;
    },

    async close() {
      if (state.pool) {
        await state.pool.end();
        state.pool = null;
        prepared.clear();
      }
    },

    get raw() { return pool; },
  };

  return adapter;
}
