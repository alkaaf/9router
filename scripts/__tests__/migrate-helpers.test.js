#!/usr/bin/env node
/**
 * Unit tests for the migration helper functions.
 *
 * Run with: node scripts/__tests__/migrate-helpers.test.js
 *
 * Tests the type-conversion and batching logic that does NOT
 * require a live SQLite or PostgreSQL connection.
 */

import assert from "node:assert/strict";
import {
  toJsonbString,
  toDbBool,
  shouldSkipColumn,
  batchRows,
} from "../migrate-sqlite-to-postgres.js";

let passed = 0;
let failed = 0;

function test(name, fn) {
  try {
    fn();
    console.log(`  ok  ${name}`);
    passed++;
  } catch (err) {
    console.error(`  FAIL ${name}: ${err.message}`);
    failed++;
  }
}

console.log("\nmigrate-helpers.test.js\n");

// ---------- toJsonbString ----------

test("toJsonbString: object is JSON-stringified", () => {
  const input = { foo: "bar", n: 42 };
  const result = toJsonbString(input);
  assert.equal(typeof result, "string");
  assert.equal(result, JSON.stringify(input));
});

test("toJsonbString: array is JSON-stringified", () => {
  const result = toJsonbString([1, 2, 3]);
  assert.equal(result, "[1,2,3]");
});

test("toJsonbString: primitive string passes through", () => {
  assert.equal(toJsonbString("hello"), "hello");
});

test("toJsonbString: number passes through", () => {
  assert.equal(toJsonbString(42), 42);
});

test("toJsonbString: null returns null", () => {
  assert.equal(toJsonbString(null), null);
});

test("toJsonbString: undefined returns null", () => {
  assert.equal(toJsonbString(undefined), null);
});

// ---------- toDbBool ----------

test("toDbBool: integer 0 returns false", () => {
  assert.equal(toDbBool(0), false);
});

test("toDbBool: integer 1 returns true", () => {
  assert.equal(toDbBool(1), true);
});

test("toDbBool: string '0' returns false", () => {
  assert.equal(toDbBool("0"), false);
});

test("toDbBool: string '1' returns true", () => {
  assert.equal(toDbBool("1"), true);
});

test("toDbBool: null returns false (Boolean coercion)", () => {
  assert.equal(toDbBool(null), false);
});

test("toDbBool: true stays true", () => {
  assert.equal(toDbBool(true), true);
});

// ---------- shouldSkipColumn ----------

test("shouldSkipColumn: skips 'id' for usageHistory (BIGSERIAL)", () => {
  assert.equal(shouldSkipColumn("id", "usageHistory"), true);
});

test("shouldSkipColumn: does NOT skip 'id' for providerConnections (TEXT PK)", () => {
  assert.equal(shouldSkipColumn("id", "providerConnections"), false);
});

test("shouldSkipColumn: does NOT skip 'id' for apiKeys (TEXT PK)", () => {
  assert.equal(shouldSkipColumn("id", "apiKeys"), false);
});

test("shouldSkipColumn: does NOT skip non-id columns", () => {
  assert.equal(shouldSkipColumn("data", "usageHistory"), false);
  assert.equal(shouldSkipColumn("provider", "providerConnections"), false);
});

// ---------- batchRows ----------

test("batchRows: chunks array of 250 into 3 batches of 100/100/50", () => {
  const rows = Array.from({ length: 250 }, (_, i) => i);
  const batches = batchRows(rows, 100);
  assert.equal(batches.length, 3);
  assert.equal(batches[0].length, 100);
  assert.equal(batches[1].length, 100);
  assert.equal(batches[2].length, 50);
});

test("batchRows: array smaller than size returns one batch", () => {
  const rows = [1, 2, 3];
  const batches = batchRows(rows, 100);
  assert.equal(batches.length, 1);
  assert.equal(batches[0].length, 3);
});

test("batchRows: exact multiple of size returns clean chunks", () => {
  const rows = Array.from({ length: 200 }, (_, i) => i);
  const batches = batchRows(rows, 100);
  assert.equal(batches.length, 2);
  assert.equal(batches[0].length, 100);
  assert.equal(batches[1].length, 100);
});

test("batchRows: empty array returns empty array", () => {
  const batches = batchRows([], 100);
  assert.equal(batches.length, 0);
});

test("batchRows: size of 1 produces N single-element batches", () => {
  const rows = [1, 2, 3, 4];
  const batches = batchRows(rows, 1);
  assert.equal(batches.length, 4);
  assert.equal(batches[0].length, 1);
  assert.deepEqual(batches[3], [4]);
});

// ---------- summary ----------

console.log(`\n${passed} passed, ${failed} failed\n`);
process.exit(failed > 0 ? 1 : 0);
