import { getAdapter } from "../driver.js";
import { parseJson, stringifyJson } from "../helpers/jsonCol.js";

const DEFAULT_MITM_ROUTER_BASE = "http://localhost:20128";
const SETTINGS_CACHE_TTL_MS = 5000;

const DEFAULT_SETTINGS = {
  cloudEnabled: false,
  tunnelEnabled: false,
  tunnelUrl: "",
  tunnelProvider: "cloudflare",
  tailscaleEnabled: false,
  tailscaleUrl: "",
  stickyRoundRobinLimit: 3,
  providerStrategies: {},
  comboStrategy: "fallback",
  comboStickyRoundRobinLimit: 1,
  comboStrategies: {},
  requireLogin: true,
  tunnelDashboardAccess: true,
  authMode: "password",
  oidcIssuerUrl: "",
  oidcClientId: "",
  oidcClientSecret: "",
  oidcScopes: "openid profile email",
  oidcLoginLabel: "Sign in with OIDC",
  enableObservability: true,
  observabilityMaxRecords: 1000,
  observabilityBatchSize: 20,
  observabilityFlushIntervalMs: 5000,
  observabilityMaxJsonSize: 5,
  outboundProxyEnabled: false,
  outboundProxyUrl: "",
  outboundNoProxy: "",
  mitmRouterBaseUrl: DEFAULT_MITM_ROUTER_BASE,
  dnsToolEnabled: {},
  rtkEnabled: true,
  cavemanEnabled: false,
  cavemanLevel: "full",
};

if (!global._settingsCache) global._settingsCache = { data: null, ts: 0 };
const settingsCache = global._settingsCache;

async function readRaw() {
  const db = await getAdapter();
  const row = await db.get(`SELECT data FROM settings WHERE id = 1`);
  if (!row) return {};
  return db.driver === "postgres" ? (row.data ?? {}) : parseJson(row.data, {});
}

function mergeWithDefaults(raw) {
  const merged = { ...DEFAULT_SETTINGS, ...(raw || {}) };
  for (const [key, defVal] of Object.entries(DEFAULT_SETTINGS)) {
    if (merged[key] === undefined) {
      if (
        key === "outboundProxyEnabled" &&
        typeof merged.outboundProxyUrl === "string" &&
        merged.outboundProxyUrl.trim()
      ) {
        merged[key] = true;
      } else {
        merged[key] = defVal;
      }
    }
  }
  return merged;
}

export async function getSettings() {
  if (settingsCache.data && Date.now() - settingsCache.ts < SETTINGS_CACHE_TTL_MS) {
    return settingsCache.data;
  }
  const raw = await readRaw();
  const merged = mergeWithDefaults(raw);
  settingsCache.data = merged;
  settingsCache.ts = Date.now();
  return merged;
}

export async function updateSettings(updates) {
  const db = await getAdapter();
  const isPg = db.driver === "postgres";
  let next;
  await db.transaction(async (txn) => {
    const row = await txn.get(`SELECT data FROM settings WHERE id = 1`);
    const current = row
      ? (isPg ? (row.data ?? {}) : parseJson(row.data, {}))
      : {};
    next = { ...current, ...updates };
    const value = isPg ? next : stringifyJson(next);
    await txn.run(
      `INSERT INTO settings(id, data) VALUES(1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
      [value]
    );
  });
  settingsCache.data = mergeWithDefaults(next);
  settingsCache.ts = Date.now();
  return settingsCache.data;
}

export async function isCloudEnabled() {
  const settings = await getSettings();
  return settings.cloudEnabled === true;
}

export async function getCloudUrl() {
  const settings = await getSettings();
  return (
    settings.cloudUrl ||
    process.env.CLOUD_URL ||
    process.env.NEXT_PUBLIC_CLOUD_URL ||
    ""
  );
}

export async function exportSettings() {
  return await readRaw();
}
