// PostgreSQL schema for 9Router.
//
// Mirrors the SQLite schema in ./schema.js. Tables, columns, and indexes
// match the source-of-truth definitions; types are mapped per the imp4
// strategy (TIMESTAMPTZ for timestamps, JSONB for JSON blobs, BOOLEAN for
// 0/1 flags, BIGSERIAL for auto-increment, NUMERIC(12,6) for cost/price).
//
// Usage:
//   import { getPostgresSchema, buildCreateTableSql } from "./schema.postgres.js";
//   const sql = getPostgresSchema();
//   // or build a single table:
//   const settings = buildSettingsTable();

export const POSTGRES_SCHEMA_VERSION = 1;

// Returns the full DDL as a single string. Each statement is separated
// by a blank line for readability. All statements use IF NOT EXISTS for
// idempotency.
export function getPostgresSchema() {
  return [
    buildMetaTable(),
    buildSettingsTable(),
    buildProviderConnectionsTable(),
    buildProviderConnectionsIndexes(),
    buildProviderNodesTable(),
    buildProviderNodesIndexes(),
    buildProxyPoolsTable(),
    buildProxyPoolsIndexes(),
    buildApiKeysTable(),
    buildApiKeysIndexes(),
    buildCombosTable(),
    buildCombosIndexes(),
    buildKvTable(),
    buildKvIndexes(),
    buildUsageHistoryTable(),
    buildUsageHistoryIndexes(),
    buildUsageDailyTable(),
    buildUsageDailyByProviderTable(),
    buildUsageDailyByModelTable(),
    buildUsageDailyByApiKeyTable(),
    buildUsageDailyByAccountTable(),
    buildUsageDailyByEndpointTable(),
    buildRequestDetailsTable(),
    buildRequestDetailsIndexes(),
  ].filter(Boolean).map((s) => s.endsWith(";") ? s : s + ";").join("\n\n") + "\n";
}

// 1. _meta — key/value store with version tracking
export function buildMetaTable() {
  return `CREATE TABLE IF NOT EXISTS _meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
)`;
}

// 2. settings — single-row app settings (id CHECK (id = 1))
export function buildSettingsTable() {
  return `CREATE TABLE IF NOT EXISTS settings (
  id   INTEGER PRIMARY KEY CHECK (id = 1),
  data JSONB NOT NULL
)`;
}

// 3. providerConnections — provider credential configs
export function buildProviderConnectionsTable() {
  return `CREATE TABLE IF NOT EXISTS providerConnections (
  id         TEXT PRIMARY KEY,
  provider   TEXT NOT NULL,
  authType   TEXT NOT NULL,
  name       TEXT,
  email      TEXT,
  priority   INTEGER,
  isActive   BOOLEAN DEFAULT TRUE,
  data       JSONB NOT NULL,
  createdAt  TIMESTAMPTZ NOT NULL,
  updatedAt  TIMESTAMPTZ NOT NULL
)`;
}

export function buildProviderConnectionsIndexes() {
  return [
    "CREATE INDEX IF NOT EXISTS idx_pc_provider ON providerConnections(provider)",
    "CREATE INDEX IF NOT EXISTS idx_pc_provider_active ON providerConnections(provider, isActive)",
    "CREATE INDEX IF NOT EXISTS idx_pc_priority ON providerConnections(provider, priority)",
  ].join(";\n");
}

// 4. providerNodes — LLM node configs
export function buildProviderNodesTable() {
  return `CREATE TABLE IF NOT EXISTS providerNodes (
  id        TEXT PRIMARY KEY,
  type      TEXT,
  name      TEXT,
  data      JSONB NOT NULL,
  createdAt TIMESTAMPTZ NOT NULL,
  updatedAt TIMESTAMPTZ NOT NULL
)`;
}

export function buildProviderNodesIndexes() {
  return "CREATE INDEX IF NOT EXISTS idx_pn_type ON providerNodes(type)";
}

// 5. proxyPools — proxy pool configs
export function buildProxyPoolsTable() {
  return `CREATE TABLE IF NOT EXISTS proxyPools (
  id         TEXT PRIMARY KEY,
  isActive   BOOLEAN DEFAULT TRUE,
  testStatus TEXT,
  data       JSONB NOT NULL,
  createdAt  TIMESTAMPTZ NOT NULL,
  updatedAt  TIMESTAMPTZ NOT NULL
)`;
}

export function buildProxyPoolsIndexes() {
  return [
    "CREATE INDEX IF NOT EXISTS idx_pp_active ON proxyPools(isActive)",
    "CREATE INDEX IF NOT EXISTS idx_pp_status ON proxyPools(testStatus)",
  ].join(";\n");
}

// 6. apiKeys — API key records
export function buildApiKeysTable() {
  return `CREATE TABLE IF NOT EXISTS apiKeys (
  id        TEXT PRIMARY KEY,
  key       TEXT UNIQUE NOT NULL,
  name      TEXT,
  machineId TEXT,
  isActive  BOOLEAN DEFAULT TRUE,
  createdAt TIMESTAMPTZ NOT NULL
)`;
}

export function buildApiKeysIndexes() {
  return "CREATE INDEX IF NOT EXISTS idx_ak_key ON apiKeys(key)";
}

// 7. combos — model combos
export function buildCombosTable() {
  return `CREATE TABLE IF NOT EXISTS combos (
  id        TEXT PRIMARY KEY,
  name      TEXT UNIQUE NOT NULL,
  kind      TEXT,
  models    JSONB NOT NULL,
  createdAt TIMESTAMPTZ NOT NULL,
  updatedAt TIMESTAMPTZ NOT NULL
)`;
}

export function buildCombosIndexes() {
  return "CREATE INDEX IF NOT EXISTS idx_combo_name ON combos(name)";
}

// 8. kv — generic key-value store with composite PK (value is JSONB for native object storage)
export function buildKvTable() {
  return `CREATE TABLE IF NOT EXISTS kv (
  scope TEXT NOT NULL,
  key   TEXT NOT NULL,
  value JSONB NOT NULL,
  PRIMARY KEY (scope, key)
)`;
}

export function buildKvIndexes() {
  return "CREATE INDEX IF NOT EXISTS idx_kv_scope ON kv(scope)";
}

// 9. usageHistory — append-only request usage log (highest write rate)
export function buildUsageHistoryTable() {
  return `CREATE TABLE IF NOT EXISTS usageHistory (
  id               BIGSERIAL PRIMARY KEY,
  timestamp        TIMESTAMPTZ NOT NULL,
  provider         TEXT,
  model            TEXT,
  connectionId     TEXT,
  apiKey           TEXT,
  endpoint         TEXT,
  promptTokens     INTEGER DEFAULT 0,
  completionTokens INTEGER DEFAULT 0,
  cost             NUMERIC(12,6) DEFAULT 0,
  status           TEXT,
  tokens           JSONB,
  meta             JSONB
)`;
}

export function buildUsageHistoryIndexes() {
  return [
    "CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(timestamp DESC)",
    "CREATE INDEX IF NOT EXISTS idx_uh_id_desc ON usageHistory(id DESC)",
    "CREATE INDEX IF NOT EXISTS idx_uh_provider ON usageHistory(provider)",
    "CREATE INDEX IF NOT EXISTS idx_uh_model ON usageHistory(model)",
    "CREATE INDEX IF NOT EXISTS idx_uh_conn ON usageHistory(connectionId)",
    "CREATE INDEX IF NOT EXISTS idx_uh_apiKey ON usageHistory(apiKey)",
    "CREATE INDEX IF NOT EXISTS idx_uh_provider_ts ON usageHistory(provider, timestamp DESC)",
    "CREATE INDEX IF NOT EXISTS idx_uh_model_ts ON usageHistory(model, timestamp DESC)",
    "CREATE INDEX IF NOT EXISTS idx_uh_conn_ts ON usageHistory(connectionId, timestamp DESC)",
    "CREATE INDEX IF NOT EXISTS idx_uh_apiKey_ts ON usageHistory(apiKey, timestamp DESC)",
  ].join(";\n");
}

// 10. usageDaily — daily aggregated usage (JSONB blob keyed by date)
export function buildUsageDailyTable() {
  return `CREATE TABLE IF NOT EXISTS usageDaily (
  dateKey DATE PRIMARY KEY,
  data    JSONB NOT NULL
)`;
}

// 10a. usageDailyByProvider — normalized rollup by (date, provider)
export function buildUsageDailyByProviderTable() {
  return `CREATE TABLE IF NOT EXISTS usageDailyByProvider (
  date         DATE NOT NULL,
  provider     TEXT NOT NULL,
  requestCount BIGINT NOT NULL DEFAULT 0,
  inputTokens  BIGINT NOT NULL DEFAULT 0,
  outputTokens BIGINT NOT NULL DEFAULT 0,
  totalTokens  BIGINT NOT NULL DEFAULT 0,
  cost         NUMERIC(12,6) NOT NULL DEFAULT 0,
  updatedAt    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (date, provider)
)`;
}

// 10b. usageDailyByModel — normalized rollup by (date, model)
export function buildUsageDailyByModelTable() {
  return `CREATE TABLE IF NOT EXISTS usageDailyByModel (
  date         DATE NOT NULL,
  model        TEXT NOT NULL,
  requestCount BIGINT NOT NULL DEFAULT 0,
  inputTokens  BIGINT NOT NULL DEFAULT 0,
  outputTokens BIGINT NOT NULL DEFAULT 0,
  totalTokens  BIGINT NOT NULL DEFAULT 0,
  cost         NUMERIC(12,6) NOT NULL DEFAULT 0,
  updatedAt    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (date, model)
)`;
}

// 10c. usageDailyByApiKey — normalized rollup by (date, apiKeyId)
export function buildUsageDailyByApiKeyTable() {
  return `CREATE TABLE IF NOT EXISTS usageDailyByApiKey (
  date         DATE NOT NULL,
  apiKeyId     TEXT NOT NULL,
  requestCount BIGINT NOT NULL DEFAULT 0,
  inputTokens  BIGINT NOT NULL DEFAULT 0,
  outputTokens BIGINT NOT NULL DEFAULT 0,
  totalTokens  BIGINT NOT NULL DEFAULT 0,
  cost         NUMERIC(12,6) NOT NULL DEFAULT 0,
  updatedAt    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (date, apiKeyId)
)`;
}

// 10d. usageDailyByAccount — normalized rollup by (date, accountId)
export function buildUsageDailyByAccountTable() {
  return `CREATE TABLE IF NOT EXISTS usageDailyByAccount (
  date         DATE NOT NULL,
  accountId    TEXT NOT NULL,
  requestCount BIGINT NOT NULL DEFAULT 0,
  inputTokens  BIGINT NOT NULL DEFAULT 0,
  outputTokens BIGINT NOT NULL DEFAULT 0,
  totalTokens  BIGINT NOT NULL DEFAULT 0,
  cost         NUMERIC(12,6) NOT NULL DEFAULT 0,
  updatedAt    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (date, accountId)
)`;
}

// 10e. usageDailyByEndpoint — normalized rollup by (date, endpoint)
export function buildUsageDailyByEndpointTable() {
  return `CREATE TABLE IF NOT EXISTS usageDailyByEndpoint (
  date         DATE NOT NULL,
  endpoint     TEXT NOT NULL,
  requestCount BIGINT NOT NULL DEFAULT 0,
  inputTokens  BIGINT NOT NULL DEFAULT 0,
  outputTokens BIGINT NOT NULL DEFAULT 0,
  totalTokens  BIGINT NOT NULL DEFAULT 0,
  cost         NUMERIC(12,6) NOT NULL DEFAULT 0,
  updatedAt    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (date, endpoint)
)`;
}

// 11. requestDetails — per-request detail logs
export function buildRequestDetailsTable() {
  return `CREATE TABLE IF NOT EXISTS requestDetails (
  id           TEXT PRIMARY KEY,
  timestamp    TIMESTAMPTZ NOT NULL,
  provider     TEXT,
  model        TEXT,
  connectionId TEXT,
  status       TEXT,
  data         JSONB NOT NULL
)`;
}

export function buildRequestDetailsIndexes() {
  return [
    "CREATE INDEX IF NOT EXISTS idx_rd_ts ON requestDetails(timestamp DESC)",
    "CREATE INDEX IF NOT EXISTS idx_rd_provider ON requestDetails(provider)",
    "CREATE INDEX IF NOT EXISTS idx_rd_model ON requestDetails(model)",
    "CREATE INDEX IF NOT EXISTS idx_rd_conn ON requestDetails(connectionId)",
  ].join(";\n");
}

// Generic builder — given a name and column/constraint definition, returns
// a CREATE TABLE statement. Mirrors buildCreateTableSql() in schema.js for
// parity with the SQLite side. Indexes are not handled here; emit them
// separately.
export function buildCreateTableSql(name, def) {
  const cols = Object.entries(def.columns || {}).map(([k, v]) => `${k} ${v}`);
  if (def.primaryKey) cols.push(def.primaryKey);
  return `CREATE TABLE IF NOT EXISTS ${name} (${cols.join(", ")})`;
}
