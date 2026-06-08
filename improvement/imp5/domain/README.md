# 9Router Backend Rewrite — Domain Breakdown

**Version:** 1.0
**Date:** 2026-06-04
**Goal:** Atomic task breakdown dari manifesto untuk rewrite Node.js → Go + Fiber + GORM

---

## Overview

| Domain | File | Tasks | Est. Days |
|--------|-------|-------|-----------|
| Database | `database.md` | DB-001–037 (37 tasks) | 3-4 |
| Chat/Core | `chat-core.md` | CHAT-001–038 (38 tasks) | 6-8 |
| Executors | `executors.md` | EXEC-001–028 (28 tasks) | 12-16 |
| Usage | `usage.md` | USAGE-001–020 (20 tasks) | 3-4 |
| Combos | `combos.md` | COMBO-001–100 (17 tasks) | 2-3 |
| OAuth & Integration | `oauth-integration.md` | OAUTH-001–021 (21 tasks) | 3 |
| Settings & System | `settings-system.md` | SYS-001–034 (34 tasks) | 3-4 |
| Providers | `providers.md` | PROV-001–025 (25 tasks) | 3-4 |
| Auth | `auth.md` | AUTH-001–020 (20 tasks) | 2-3 |
| Tunnel | `tunnel.md` | TUNNEL-001–012 (12 tasks) | 2-3 |
| **TOTAL** | | **~252 tasks** | **38-50 days** |

---

## Domain Dependencies

```
Phase 0: Reverse Engineering & Spec (WEEK 1-2)
│
├─ READ manifest.md ──────────────────────────────────────────────┐
│   All domains harus baca manifesto dulu                          │
└────────────────────────────────────────────────────────────────┘
                    │
                    ▼
Phase 1: Core Infrastructure (WEEK 3-5)
│
├─ AUTH (AUTH-001) ───────────────────────────────────────────────┐
│   JWT types, signing/verification, bcrypt helpers                │
├─ DATABASE (DB-001, DB-002, DB-008) ───────────────────────────┤
│   GORM models, connection manager, auto-migrate                 │
└────────────────────────────────────────────────────────────────┘
                    │
                    ▼
Phase 2: REST API Implementation (WEEK 6-8)
│
├─ SETTINGS-SYSTEM ──────────────────────────────────────────────┐
│   SYS-001–006: Settings core (depends on DB)                   │
│   SYS-007–012: Health/Init/Version/Shutdown                   │
│   SYS-013–014: Locale/Tags                                    │
│   SYS-015–017: Pricing                                       │
│   SYS-018–026: Models                                        │
│   SYS-027–029: Provider Nodes                                │
│   SYS-030–034: Proxy Pools                                   │
├─ PROVIDERS ────────────────────────────────────────────────────┤
│   PROV-001–005: Model resolution (depends on DB)              │
│   PROV-006–010: CRUD endpoints                               │
│   PROV-011–015: Validation                                   │
│   PROV-016: Batch testing                                    │
│   PROV-017–024: Credentials & selection                       │
│   PROV-025: Suggested models                                  │
├─ APIKEYS ─────────────────────────────────────────────────────┤
│   (part of SETTINGS-SYSTEM)                                   │
├─ AUTH (AUTH-002–AUTH-020) ───────────────────────────────────┤
│   Login/logout, middleware, OIDC                              │
└────────────────────────────────────────────────────────────────┘
                    │
                    ▼
Phase 3: AI Proxy Engine (WEEK 9-12)
│
├─ EXECUTORS ────────────────────────────────────────────────────┐
│   EXEC-001–002: Base interface + registry                     │
│   EXEC-003–005: OpenAI-compatible group (github, grok, etc)  │
│   EXEC-006–010: OAuth-based group (antigravity, kiro, etc)   │
│   EXEC-011–017: Custom format group (azure, vertex, etc)     │
│   EXEC-018–023: Translators                                  │
│   EXEC-024–028: RTK filters                                 │
├─ CHAT/CORE ───────────────────────────────────────────────────┤
│   CHAT-001–005: Request parsing (depends on AUTH, DB)         │
│   CHAT-006–010: Model resolution                            │
│   CHAT-011–013: Bypass handler                               │
│   CHAT-014–021: Credential rotation & fallback                │
│   CHAT-022–026: Combo orchestration                          │
│   CHAT-027–034: Streaming SSE + Usage tracking                │
│   CHAT-035–038: Handler entry point                         │
├─ COMBOS ──────────────────────────────────────────────────────┤
│   COMBO-001–017: All combo tasks (depends on CHAT/core)      │
└────────────────────────────────────────────────────────────────┘
                    │
                    ▼
Phase 3.5: Media & Integration (WEEK 12-13)
│
├─ OAUTH-INTEGRATION ───────────────────────────────────────────┐
│   OAUTH-001–009: OAuth flows per provider                    │
│   OAUTH-010–011: MCP handlers                                │
│   OAUTH-012–014: CLI tools settings                          │
│   OAUTH-015–017: Translator backend                          │
│   OAUTH-018–021: Media providers                            │
├─ TUNNEL ─────────────────────────────────────────────────────┤
│   TUNNEL-001–012: Cloudflare + Tailscale subprocess mgmt    │
└────────────────────────────────────────────────────────────────┘
                    │
                    ▼
Phase 4: Streaming SSE (WEEK 13-14)
│
├─ CHAT/STREAMING ─────────────────────────────────────────────┐
│   CHAT-027–031: SSE writer (depends on EXECUTORS)            │
│   Tool calls streaming (depends on CHAT-014–021)            │
└────────────────────────────────────────────────────────────────┘
                    │
                    ▼
Phase 5: Migration Strategy (WEEK 15-16)
│
├─ Parallel run dengan Nginx proxy                              │
├─ Gradual cutover                                              │
└─ Cleanup                                                      │
                    │
                    ▼
Phase 6: Testing & QA (ONGOING)
│
├─ Unit tests per domain (see each domain file)                │
├─ Integration tests                                            │
├─ E2E tests (Playwright)                                      │
└─ Contract tests (byte-by-byte comparison with Node.js)        │
```

---

## Critical Path (Minimum Viable Rewrite)

Untuk dapat MVP working, implementasi dalam urutan:

1. **DB-001** → GORM models + connection
2. **AUTH-001** → JWT + bcrypt helpers
3. **DB-002** → Auto-migrate
4. **AUTH-002** → Auth middleware
5. **SYS-001–006** → Settings GET/PATCH
6. **SYS-007** → Health endpoint
7. **PROV-006–010** → Provider CRUD
8. **EXEC-001–002** → Executor interface + registry
9. **EXEC-003** → OpenAI executor (reference)
10. **CHAT-001–005** → Request parsing
11. **CHAT-006–010** → Model resolution
12. **CHAT-027–031** → SSE streaming
13. **CHAT-014–021** → Credential rotation
14. **COMBO-001–017** → Combo strategies

**MVP Total: ~40 tasks, ~8-10 weeks**

---

## Test Coverage per Domain

| Domain | Unit Tests | Integration Tests | E2E Tests |
|-------|-----------|-------------------|------------|
| Auth | 20 | 10 | 5 |
| Database | 130 | 15 | 5 |
| Providers | 30 | 20 | 10 |
| Combos | 40 | 15 | 8 |
| Chat/Core | 50 | 25 | 10 |
| Executors | 60 | 30 | 15 |
| Usage | 16 | 9 | 5 |
| Settings | 25 | 15 | 5 |
| Tunnel | 15 | 20 | 10 |
| OAuth | 20 | 15 | 5 |
| **TOTAL** | **~406** | **~174** | **~78** |

---

## File Locations

```
improvement/imp5/
├── manifest.md              # Master blueprint (1936 lines)
└── domain/
    ├── README.md           # This file
    ├── auth.md             # Auth domain (20 tasks)
    ├── database.md         # Database/GORM domain (37 tasks)
    ├── providers.md        # Provider domain (25 tasks)
    ├── combos.md           # Combo domain (17 tasks)
    ├── chat-core.md        # Chat/Core domain (38 tasks)
    ├── executors.md        # Executors domain (28 tasks)
    ├── usage.md            # Usage tracking domain (20 tasks)
    ├── settings-system.md   # Settings & System domain (34 tasks)
    ├── tunnel.md           # Tunnel domain (12 tasks)
    └── oauth-integration.md # OAuth & Integration domain (21 tasks)
```

---

## Notes

- Task ID prefixes match: `AUTH`, `DB`, `PROV`, `COMBO`, `CHAT`, `EXEC`, `USAGE`, `SYS`, `TUNNEL`, `OAUTH`
- Dependencies noted per task dalam masing-masing domain file
- Coverage targets: 85% minimum per package
- E2E testing dengan Playwright untuk critical flows
- Contract tests untuk verify Go = Node.js byte-by-byte

---

*Generated: 2026-06-04*
*Version: 1.0*
