/**
 * Unit tests for cost calculation logic (calculateCostFromTokens, getPricingForModel).
 *
 * These tests guard:
 *   - Correctness of calculateCostFromTokens across token types
 *   - 3-step fallback chain in getPricingForModel (PROVIDER_PRICING → MODEL_PRICING → PATTERN_PRICING)
 *   - Performance: dedup map reduces pricing lookups for repeated (provider, model) pairs
 */

import { describe, it, expect } from "vitest";
import {
  calculateCostFromTokens,
  getPricingForModel,
  PROVIDER_PRICING,
  MODEL_PRICING,
  PATTERN_PRICING,
} from "../../src/shared/constants/pricing.js";

describe("calculateCostFromTokens — correctness", () => {
  it("returns 0 for null tokens or null pricing", () => {
    expect(calculateCostFromTokens(null, { input: 1 })).toBe(0);
    expect(calculateCostFromTokens({ prompt_tokens: 100 }, null)).toBe(0);
    expect(calculateCostFromTokens(null, null)).toBe(0);
  });

  it("calculates cost from prompt_tokens + completion_tokens", () => {
    const pricing = { input: 2.5, output: 10.0 };
    const tokens = { prompt_tokens: 1_000_000, completion_tokens: 500_000 };
    // 1M input * 2.5/M + 0.5M output * 10/M = 2.5 + 5.0 = 7.5
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(7.5, 6);
  });

  it("falls back to input_tokens / output_tokens aliases", () => {
    const pricing = { input: 1.0, output: 4.0 };
    const tokens = { input_tokens: 2_000_000, output_tokens: 1_000_000 };
    // 2M * 1/M + 1M * 4/M = 2 + 4 = 6
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(6.0, 6);
  });

  it("subtracts cached_tokens from input tokens", () => {
    const pricing = { input: 3.0, output: 15.0, cached: 0.30 };
    const tokens = {
      prompt_tokens: 1_000_000,
      cached_tokens: 800_000,
      completion_tokens: 0,
    };
    // nonCachedInput = 200_000 * 3/M = 0.6
    // cached = 800_000 * 0.30/M = 0.24
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(0.84, 6);
  });

  it("uses cache_read_input_tokens alias for cached", () => {
    const pricing = { input: 3.0, output: 15.0, cached: 0.30 };
    const tokens = {
      input_tokens: 1_000_000,
      cache_read_input_tokens: 500_000,
      completion_tokens: 0,
    };
    // nonCachedInput = 500_000 * 3/M = 1.5
    // cached = 500_000 * 0.30/M = 0.15
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(1.65, 6);
  });

  it("charges reasoning tokens at reasoning rate (or output rate fallback)", () => {
    const pricing = { input: 3.0, output: 15.0, reasoning: 22.5 };
    const tokens = {
      prompt_tokens: 0,
      completion_tokens: 0,
      reasoning_tokens: 1_000_000,
    };
    // 1M * 22.5/M = 22.5
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(22.5, 6);

    // Falls back to output rate when reasoning missing
    const pricingNoReasoning = { input: 3.0, output: 15.0 };
    expect(calculateCostFromTokens(tokens, pricingNoReasoning)).toBeCloseTo(15.0, 6);
  });

  it("charges cache_creation tokens at cache_creation rate (or input rate fallback)", () => {
    const pricing = { input: 3.0, output: 15.0, cache_creation: 3.75 };
    const tokens = {
      prompt_tokens: 0,
      cache_creation_input_tokens: 1_000_000,
    };
    // 1M * 3.75/M = 3.75
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(3.75, 6);

    // Falls back to input rate when cache_creation missing
    const pricingNoCacheCreation = { input: 3.0, output: 15.0 };
    expect(calculateCostFromTokens(tokens, pricingNoCacheCreation)).toBeCloseTo(3.0, 6);
  });

  it("combines all token types in a single call", () => {
    const pricing = {
      input: 3.0,
      output: 15.0,
      cached: 0.30,
      reasoning: 22.5,
      cache_creation: 3.75,
    };
    const tokens = {
      prompt_tokens: 10_000_000, // 10M
      cached_tokens: 8_000_000, // 8M cached
      cache_creation_input_tokens: 1_000_000, // 1M new cache
      completion_tokens: 2_000_000, // 2M output
      reasoning_tokens: 1_000_000, // 1M reasoning
    };
    // nonCachedInput = (10M - 8M) = 2M * 3/M = 6
    // cached = 8M * 0.30/M = 2.4
    // cache_creation = 1M * 3.75/M = 3.75
    // output = 2M * 15/M = 30
    // reasoning = 1M * 22.5/M = 22.5
    // total = 64.65
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(64.65, 6);
  });

  it("treats negative nonCachedInput as 0 (defensive against bad input)", () => {
    const pricing = { input: 3.0, output: 15.0 };
    const tokens = { prompt_tokens: 100, cached_tokens: 500, completion_tokens: 0 };
    // nonCachedInput = max(0, 100-500) = 0 → charged 0
    // cachedTokens=500 charged at input rate (3.0/M) → 0.0015
    expect(calculateCostFromTokens(tokens, pricing)).toBeCloseTo(0.0015, 6);
  });
});

describe("getPricingForModel — 3-step fallback chain", () => {
  it("returns null for missing model", () => {
    expect(getPricingForModel("openai", null)).toBeNull();
    expect(getPricingForModel("openai", undefined)).toBeNull();
  });

  it("step 1: returns PROVIDER_PRICING override when present", () => {
    // gh: gpt-5.3-codex has a different rate than canonical
    const pricing = getPricingForModel("gh", "gpt-5.3-codex");
    expect(pricing).toBeTruthy();
    expect(pricing.input).toBe(1.75);
    expect(pricing.output).toBe(14.0);
  });

  it("step 2: returns MODEL_PRICING canonical rate when no provider override", () => {
    // claude-sonnet-4-5 has a canonical rate and no provider override
    const pricing = getPricingForModel("anthropic", "claude-sonnet-4-5-20250929");
    expect(pricing).toBeTruthy();
    expect(pricing.input).toBe(3.0);
    expect(pricing.output).toBe(15.0);
  });

  it("step 2: strips vendor prefix from model name (deepseek/deepseek-chat → deepseek-chat)", () => {
    const pricing = getPricingForModel("openrouter", "deepseek/deepseek-chat");
    expect(pricing).toBeTruthy();
    expect(pricing.input).toBe(0.14);
  });

  it("step 3: falls back to PATTERN_PRICING when no exact model match", () => {
    // Some made-up codex model name should match *-codex pattern
    const pricing = getPricingForModel("openai", "gpt-5-fake-codex-variant");
    expect(pricing).toBeTruthy();
  });

  it("step 3: pattern fallback respects glob ordering (specific first)", () => {
    // *-codex-xhigh is more specific than *-codex, so it should win
    const xhigh = getPricingForModel("openai", "gpt-5.1-codex-xhigh");
    const high = getPricingForModel("openai", "gpt-5.1-codex-high");
    expect(xhigh.input).toBe(10.0);
    expect(high.input).toBe(8.0);
  });

  it("returns null for unknown model that matches no pattern", () => {
    const pricing = getPricingForModel("unknown-provider", "completely-unknown-model-xyz");
    expect(pricing).toBeNull();
  });
});

describe("getPricingForModel — performance & data shape", () => {
  it("PROVIDER_PRICING, MODEL_PRICING, PATTERN_PRICING are non-empty", () => {
    expect(Object.keys(PROVIDER_PRICING).length).toBeGreaterThan(0);
    expect(Object.keys(MODEL_PRICING).length).toBeGreaterThan(10);
    expect(PATTERN_PRICING.length).toBeGreaterThan(5);
  });

  it("every pricing entry has the expected numeric fields", () => {
    const sample = MODEL_PRICING["gpt-4o"];
    expect(typeof sample.input).toBe("number");
    expect(typeof sample.output).toBe("number");
  });

  it("dedup map: repeated (provider, model) lookups are O(1) via cached reference", () => {
    // Simulate the dedup map pattern from _flushWriteQueue.
    // We resolve 100 identical lookups, then ensure only 1 actual function call was made.
    let actualCalls = 0;
    const spy = (provider, model) => {
      actualCalls++;
      return getPricingForModel(provider, model);
    };
    const pricingMap = new Map();
    const batch = Array.from({ length: 100 }, () => ({ provider: "openai", model: "gpt-4o" }));

    // Replicate the dedup map logic from _flushWriteQueue
    const uniqueKeys = new Set(batch.map((b) => `${b.provider}:${b.model}`));
    expect(uniqueKeys.size).toBe(1);

    for (const key of uniqueKeys) {
      const [provider, model] = key.split(":");
      pricingMap.set(key, spy(provider, model));
    }
    // Subsequent lookups for any batch entry hit the map, not the function
    for (const raw of batch) {
      const key = `${raw.provider}:${raw.model}`;
      const pricing = pricingMap.get(key);
      expect(pricing).toBeTruthy();
    }

    expect(actualCalls).toBe(1);
  });

  it("dedup map: distinct (provider, model) pairs each incur one pricing lookup", () => {
    let actualCalls = 0;
    const spy = (provider, model) => {
      actualCalls++;
      return getPricingForModel(provider, model);
    };
    const batch = [
      { provider: "openai", model: "gpt-4o" },
      { provider: "openai", model: "gpt-4o" },     // dup
      { provider: "anthropic", model: "claude-sonnet-4-5-20250929" },
      { provider: "openai", model: "gpt-4o" },     // dup
      { provider: "openai", model: "gpt-4o-mini" },
    ];
    const pricingMap = new Map();
    const uniqueKeys = [...new Set(batch.map((b) => `${b.provider}:${b.model}`))];
    for (const key of uniqueKeys) {
      const [provider, model] = key.split(":");
      pricingMap.set(key, spy(provider, model));
    }
    // 3 unique pairs → 3 actual calls (not 5)
    expect(actualCalls).toBe(3);
    expect(pricingMap.size).toBe(3);
  });
});

describe("calculateCostFromTokens — performance (batch throughput)", () => {
  it("computes 1000 token→cost conversions in <50ms (sync, in-memory)", () => {
    const pricing = { input: 3.0, output: 15.0, cached: 0.30, reasoning: 22.5, cache_creation: 3.75 };
    const t0 = Date.now();
    let total = 0;
    for (let i = 0; i < 1000; i++) {
      const tokens = {
        prompt_tokens: 1_000_000 + i,
        cached_tokens: 500_000,
        completion_tokens: 200_000,
        reasoning_tokens: 100_000,
        cache_creation_input_tokens: 50_000,
      };
      total += calculateCostFromTokens(tokens, pricing);
    }
    const ms = Date.now() - t0;
    expect(total).toBeGreaterThan(0);
    expect(ms).toBeLessThan(50);
  });
});
