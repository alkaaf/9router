import { ensureDirs, DATA_FILE } from "./paths.js";
import { PRAGMA_SQL } from "./schema.js";

// Use global to survive Next.js dev hot-reload (module state resets on reload)
if (!global._dbAdapter) global._dbAdapter = { instance: null, initPromise: null, logged: false };
const state = global._dbAdapter;

async function tryPostgres() {
  const url = process.env.POSTGRES_URL;
  if (!url || url.trim() === "") return null;
  try {
    const { createPostgresAdapter } = await import("./adapters/postgresAdapter.js");
    const adapter = await createPostgresAdapter({ connectionString: url });
    // Apply schema (DDL is idempotent — uses IF NOT EXISTS)
    const { getPostgresSchema } = await import("./schema.postgres.js");
    const schema = getPostgresSchema();
    await adapter.exec(schema);
    return adapter;
  } catch (e) {
    console.warn(`[DB] PostgreSQL init failed, falling back to SQLite: ${e.message}`);
    return null;
  }
}

async function tryBunSqlite() {
  // Bun runtime only — built-in, no install needed
  if (!process.versions.bun) return null;
  try {
    const { createBunSqliteAdapter } = await import("./adapters/bunSqliteAdapter.js");
    return await createBunSqliteAdapter(DATA_FILE);
  } catch (e) {
    console.warn(`[DB] bun:sqlite unavailable: ${e.message}`);
    return null;
  }
}

async function tryBetterSqlite() {
  // Skip on Bun — better-sqlite3 native bindings unsupported
  if (process.versions.bun) return null;
  try {
    const { createBetterSqliteAdapter } = await import("./adapters/betterSqliteAdapter.js");
    return createBetterSqliteAdapter(DATA_FILE);
  } catch (e) {
    console.warn(`[DB] better-sqlite3 unavailable: ${e.message}`);
    return null;
  }
}

async function tryNodeSqlite() {
  // Built-in since Node 22.5.0 — no install needed. Skip under Bun (no node:sqlite).
  if (process.versions.bun) return null;
  const [maj, min] = process.versions.node.split(".").map(Number);
  if (maj < 22 || (maj === 22 && min < 5)) return null;
  try {
    const { createNodeSqliteAdapter } = await import("./adapters/nodeSqliteAdapter.js");
    return await createNodeSqliteAdapter(DATA_FILE);
  } catch (e) {
    console.warn(`[DB] node:sqlite unavailable: ${e.message}`);
    return null;
  }
}

async function trySqlJs() {
  try {
    const { createSqlJsAdapter } = await import("./adapters/sqljsAdapter.js");
    return await createSqlJsAdapter(DATA_FILE);
  } catch (e) {
    console.warn(`[DB] sql.js unavailable: ${e.message}`);
    return null;
  }
}

async function initAdapter() {
  ensureDirs();
  // Priority: PostgreSQL (if configured) → SQLite adapters
  let adapter = await tryPostgres();

  if (!adapter) {
    // Order per runtime when no POSTGRES_URL:
    //   Bun:  bun:sqlite → sql.js
    //   Node: better-sqlite3 → node:sqlite (≥22.5) → sql.js
    adapter = await tryBunSqlite();
  }
  if (!adapter) {
    adapter = await tryBetterSqlite();
  }
  if (!adapter) {
    adapter = await tryNodeSqlite();
  }
  if (!adapter) {
    adapter = await trySqlJs();
  }
  if (!adapter) {
    const triedPg = process.env.POSTGRES_URL && process.env.POSTGRES_URL.trim() !== "";
    if (triedPg) {
      throw new Error("[DB] PostgreSQL connection failed and no SQLite driver available");
    }
    throw new Error("[DB] No SQLite driver available (bun/better/node/sql.js all failed)");
  }

  if (!state.logged) {
    console.log(`[DB] Driver: ${adapter.driver}`);
    state.logged = true;
  }

  if (adapter.driver !== "postgres") {
    await adapter.exec(PRAGMA_SQL);
  }

  const { runMigrationOnce } = await import("./migrate.js");
  await runMigrationOnce(adapter);
  return adapter;
}

export async function getAdapter() {
  if (state.instance) return state.instance;
  if (!state.initPromise) state.initPromise = initAdapter().then((a) => { state.instance = a; return a; });
  return state.initPromise;
}

export function getAdapterSync() {
  if (!state.instance) throw new Error("[DB] adapter not initialized — await getAdapter() first");
  return state.instance;
}
