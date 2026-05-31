# Atomic Task: api-4 — Test Data Accuracy (Cross-Validation)

**Domain**: API Testing
**Priority**: High
**Estimated effort**: 30 min

---

## Input

- All 3 per-key API endpoints
- Test database with known, controlled usage data
- Direct database query capability for ground truth comparison

## Output

- Cross-validation test suite
- Report showing API data matches ground truth

## Process

1. Setup: create deterministic test data:
   - Key A: exactly 10 requests, 5000 prompt tokens, 2000 completion tokens, $1.50 cost
   - Key B: exactly 5 requests, 3000 prompt tokens, 1000 completion tokens, $0.80 cost
   - Spread across 3 days for chart testing
2. Write test cases:

### Test Cases

| # | Case | Expected |
|---|------|----------|
| 1 | `totalRequests` accuracy | `GET /per-key/{A}` → `stats.totalRequests` === 10 (exact) |
| 2 | `totalPromptTokens` accuracy | `stats.totalPromptTokens` === 5000 |
| 3 | `totalCompletionTokens` accuracy | `stats.totalCompletionTokens` === 2000 |
| 4 | `totalCost` accuracy | `stats.totalCost` === 1.50 (within $0.01 tolerance) |
| 5 | `byModel` sum matches total | Sum of all `byModel[].requests` === `stats.totalRequests` |
| 6 | `byModel` cost sum matches | Sum of all `byModel[].cost` === `stats.totalCost` |
| 7 | Chart `tokens` matches stats | Sum of `chartData[].tokens` ≈ `stats.totalPromptTokens + totalCompletionTokens` |
| 8 | Chart `cost` matches stats | Sum of `chartData[].cost` ≈ `stats.totalCost` |
| 9 | History count matches | `history.length` === count of rows for key A in database |
| 10 | History items match DB | Each history item's fields match corresponding DB row |

## Dependencies

- api-1, api-2, api-3: All endpoints working (DONE)

## Success Criteria

- All 10 test cases pass
- No discrepancy between API response and direct DB query
- Floating point costs within acceptable tolerance
