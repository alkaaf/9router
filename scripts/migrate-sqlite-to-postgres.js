#!/usr/bin/env node
/**
 * Migrate data from SQLite to PostgreSQL.
 *
 * Usage:
 *   POSTGRES_URL="postgres://user:pass@host:5432/db" node scripts/migrate-sqlite-to-postgres.js
 *
 * Tables are migrated in dependency order. Already-migrated rows are
 * skipped (idempotent via INSERT ... ON CONFLICT DO NOTHING).
 *
 * CLI flags:
 *   --dry-run        print what would be done, do not write
 *   --tables=x,y     only migrate these tables (default: all)
 *   --batch-size=N   rows per batch insert (default: 200)
 */

import Database from "better-sqlite3";
import pg from "pg";
import path from "node:path";
import fs from "node:fs";

const { Pool } = pg;

// ---------- helpers ----------

/**
 * Convert a JS value to a JSONB-safe string.
 * Objects/arrays are JSON-stringified so pg can parse them as JSONB.
 * Primitives are returned as-is (numbers, strings, null).
 */
export function toJsonbString(val) {
  if (val === null || val === undefined) return null;
  if (typeof val === "object") return JSON.stringify(val);
  return val;
}

/**
 * Convert SQLite INTEGER 0/1 to a JS boolean for PG BOOLEAN columns.
 */
export function toDbBool(val) {
  if (val === 0 || val === "0") return false;
  if (val === 1 || val === "1") return true;
  return Boolean(val);
}

/**
 * Whether to skip a column when building an INSERT statement.
 * Currently only skips 'id' on tables where PG uses BIGSERIAL
 * (usageHistory — the SQLite-side AUTOINCREMENT id is omitted so pg allocates it).
 */
export function shouldSkipColumn(col, table) {
  return col === "id" && table === "usageHistory";
}

/**
 * Split an array into chunks of `size`.
 */
export function batchRows(rows, size) {
  const batches = [];
  for (let i = 0; i < rows.length; i += size) {
    batches.push(rows.slice(i, i + size));
  }
  return batches;
}

// ---------- CLI argument parsing ----------

function parseArgs(argv) {
  const opts = {
    dryRun: false,
    tables: null,      // null = all tables
    batchSize: 200,
  };
  for (const arg of argv) {
    if (arg === "--dry-run") opts.dryRun = true;
    else if (arg.startsWith("--tables=")) opts.tables = arg.slice("--tables=".length).split(",");
    else if (arg.startsWith("--batch-size=")) opts.batchSize = parseInt(arg.slice("--batch-size=".length), 10);
  }
  return opts;
}

// ---------- per-table column transformers ----------

/**
 * Transform a SQLite row into PG-compatible column values.
 * Returns a { col, val } array ready for parameterized INSERT.
 */
function transformRow(table, row) {
  const result = [];
  for (const [col, raw] of Object.entries(row)) {
    if (shouldSkipColumn(col, table)) continue;

    let val = raw;

    // BIGSERIAL id already handled by shouldSkipColumn
    if (col === "id" && table !== "usageHistory") {
      // TEXT primary keys — pass as-is
      val = String(raw ?? "");
    }

    // JSONB columns: JSON-encode objects/arrays so pg can store as JSONB
    else if (
      (table === "providerConnections" && col === "data") ||
      (table === "providerNodes" && col === "data") ||
      (table === "proxyPools" && col === "data") ||
      (table === "combos" && col === "models") ||
      (table === "kv" && col === "value") ||
      (table === "usageHistory" && (col === "tokens" || col === "meta")) ||
      (table === "usageDaily" && col === "data") ||
      (table === "requestDetails" && col === "data") ||
      (table === "settings" && col === "data")
    ) {
      val = toJsonbString(raw);
    }

    // BOOLEAN columns: 0/1 integer from SQLite → JS boolean
    else if (
      (table === "providerConnections" && col === "isActive") ||
      (table === "proxyPools" && col === "isActive") ||
      (table === "apiKeys" && col === "isActive")
    ) {
      val = toDbBool(raw);
    }

    // TIMESTAMPTZ columns: TEXT ISO strings from SQLite pass through as-is
    else if (
      col === "createdAt" || col === "updatedAt" ||
      (table === "usageHistory" && col === "timestamp") ||
      (table === "requestDetails" && col === "timestamp")
    ) {
      // SQLite stores ISO strings; pg accepts them directly for TIMESTAMPTZ
      val = String(raw ?? "");
    }

    // DATE columns: YYYY-MM-DD strings pass through as-is
    else if (
      col === "dateKey" ||
      col === "date" ||
      (table === "usageDailyByProvider" && col === "date") ||
      (table === "usageDailyByModel" && col === "date") ||
      (table === "usageDailyByApiKey" && col === "date") ||
      (table === "usageDailyByAccount" && col === "date") ||
      (table === "usageDailyByEndpoint" && col === "date")
    ) {
      val = String(raw ?? "");
    }

    // NUMERIC(12,6) for cost: ensure numeric
    else if (
      table === "usageHistory" && col === "cost"
    ) {
      val = raw === null || raw === undefined ? 0 : Number(raw);
    }

    else {
      // Fallback: stringify anything that looks like it should be a string
      if (val !== null && val !== undefined && typeof val !== "string" && typeof val !== "number") {
        val = String(val);
      }
    }

    result.push({ col, val });
  }
  return result;
}

// ---------- table migration order ----------

// Tables in dependency order. UsageDailyBy* are also included.
const ALL_TABLES = [
  "_meta",
  "settings",
  "providerConnections",
  "providerNodes",
  "proxyPools",
  "apiKeys",
  "combos",
  "kv",
  "usageDaily",
  "usageDailyByProvider",
  "usageDailyByModel",
  "usageDailyByApiKey",
  "usageDailyByAccount",
  "usageDailyByEndpoint",
  "usageHistory",
  "requestDetails",
];

// ---------- migration logic ----------

async function migrateTable(pgClient, table, rows, opts) {
  if (!rows || rows.length === 0) {
    console.log(`Migrating ${table}: 0 rows (empty table)`);
    return { inserted: 0, skipped: 0 };
  }

  let inserted = 0;
  let skipped = 0;

  for (const batch of batchRows(rows, opts.batchSize)) {
    // Transform all rows in this batch
    const transformed = batch.map((row) => transformRow(table, row));

    // Build the ON CONFLICT DO NOTHING INSERT
    const cols = transformed[0].map((c) => c.col);
    const placeholders = transformed.map(
      (_, rowIdx) =>
        `(${cols.map((_, colIdx) => `$${rowIdx * cols.length + colIdx + 1}`).join(", ")})`
    ).join(", ");

    const sql = `INSERT INTO ${table} (${cols.join(", ")}) VALUES ${placeholders} ON CONFLICT DO NOTHING RETURNING 1`;

    // Flatten all values in row order for $1, $2, ...
    const params = transformed.flatMap((row) => row.map((c) => c.val));

    if (opts.dryRun) {
      console.log(`  [dry-run] INSERT ${table}: ${batch.length} rows`);
    } else {
      try {
        const result = await pgClient.query(sql, params);
        inserted += result.rowCount ?? 0;
        skipped += batch.length - (result.rowCount ?? 0);
      } catch (err) {
        throw new Error(`Failed to insert into ${table}: ${err.message}`);
      }
    }
  }

  return { inserted, skipped };
}

async function migrateAll(sqlite, pgPool, opts) {
  // Determine which tables to process
  const tablesToMigrate = opts.tables
    ? ALL_TABLES.filter((t) => opts.tables.includes(t))
    : ALL_TABLES;

  console.log(`\nStarting migration: ${tablesToMigrate.length} table(s) (batch size: ${opts.batchSize})${opts.dryRun ? " [DRY RUN]" : ""}\n`);

  let totalInserted = 0;
  let totalSkipped = 0;
  let hadError = false;

  for (const table of tablesToMigrate) {
    process.stdout.write(`Migrating ${table}: `);
    process.stdout.flush();

    try {
      // Read all rows from SQLite
      const rows = sqlite.prepare(`SELECT * FROM ${table}`).all();
      const { inserted, skipped } = await migrateTable(pgPool, table, rows, opts);
      totalInserted += inserted;
      totalSkipped += skipped;
      console.log(`${rows.length} rows → inserted ${inserted}, skipped ${skipped}`);
    } catch (err) {
      console.error(`ERROR: ${err.message}`);
      hadError = true;
      // Continue with next table as instructed
    }
  }

  console.log(`\nDone: ${totalInserted} rows inserted, ${totalSkipped} rows skipped (already existed)`);
  if (opts.dryRun) console.log("(dry run — no changes written)");

  return !hadError;
}

// ---------- main ----------

async function main() {
  const opts = parseArgs(process.argv.slice(2));

  // Validate POSTGRES_URL
  const pgUrl = process.env.POSTGRES_URL;
  if (!pgUrl) {
    console.error("Error: POSTGRES_URL environment variable is required.");
    console.error("Example: POSTGRES_URL=\"postgres://user:pass@localhost:5432/9router\" node scripts/migrate-sqlite-to-postgres.js");
    process.exit(1);
  }

  // Resolve DATA_FILE relative to the repo root (parent of scripts/)
  const repoRoot = path.resolve(__dirname, "..");
  const dataDir = path.join(repoRoot, "data");
  const dbDir = path.join(dataDir, "db");
  const dataFile = path.join(dbDir, "data.sqlite");

  if (!fs.existsSync(dataFile)) {
    console.error(`Error: SQLite file not found at ${dataFile}`);
    console.error("Run 9Router first to create the database.");
    process.exit(1);
  }

  console.log(`SQLite: ${dataFile}`);

  let sqlite;
  try {
    sqlite = new Database(dataFile, { readonly: true });
    sqlite.pragma("journal_mode = WAL");
  } catch (err) {
    console.error(`Error opening SQLite: ${err.message}`);
    process.exit(1);
  }

  // Open PostgreSQL pool (target)
  const pgPool = new Pool({
    connectionString: pgUrl,
    max: 10,
    idleTimeoutMillis: 30000,
    connectionTimeoutMillis: 10000,
  });

  try {
    // Verify PG connection
    const pgClient = await pgPool.connect();
    try {
      await pgClient.query("SELECT 1");
    } finally {
      pgClient.release();
    }
  } catch (err) {
    console.error(`Error connecting to PostgreSQL: ${err.message}`);
    await pgPool.end();
    sqlite.close();
    process.exit(1);
  }

  console.log(`PostgreSQL: ${pgUrl.replace(/:([^@/]+)@/, ":***@")} (connected)\n`);

  // Run migration
  const success = await migrateAll(sqlite, pgPool, opts);

  // Cleanup
  sqlite.close();
  await pgPool.end();

  process.exit(success ? 0 : 1);
}

// Only run main() when executed directly, not when imported for testing.
// Compare import.meta.url to the script's argv[1] path-resolved form.
import { fileURLToPath } from "node:url";
const isDirectInvocation =
  process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1]);

if (isDirectInvocation) {
  main().catch((err) => {
    console.error("Fatal error:", err);
    process.exit(1);
  });
}
