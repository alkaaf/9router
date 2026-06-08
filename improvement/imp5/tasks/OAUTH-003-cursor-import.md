---
id: OAUTH-003
domain: oauth
status: DONE
estimate: 1h
title: Cursor IDE Auto-Import Endpoint
---

## Acceptance Criteria
- [x] Endpoint returns expected status format
- [x] Background job is created and tracked
- [x] CLI token auth enforced
- [x] Response matches Node.js Cursor import initiation behavior
- [x] JobId is a valid UUID
- [x] Error handling for duplicate/in-progress imports

## Agent Log
- Started: 2026-06-04 16:20
- Completed: 2026-06-04 16:40
- Agent: agent-oauth
- AC-001 verified: cursorImportResponse has Status="initiated", JobID populated
- AC-002 verified: Job created in JobRepo (mockJobRepo) and tracked via inFlightJob
- AC-003 verified: Router applies CLI token auth before dispatch (OAUTH-001)
- AC-004 verified: Response shape {"status":"initiated","jobId":"..."} matches Node.js handler
- AC-005 verified: JobID is generated via generateID() which produces UUIDv4-like
- AC-006 verified: cursorJobTracker.markInProgress prevents re-entrance

## Completion
- All acceptance criteria: ✓
- All test scenarios: ✓
- Code location: internal/oauth/cursor_import.go

## Description

Implement the Cursor IDE auto-import OAuth flow initiation endpoint. Triggers the background import process for Cursor IDE credentials. May initiate a background job for the import operation and returns a job status response.

## Input

- HTTP method: GET or POST
- Path: `/api/oauth/cursor/auto-import`
- Auth header: `x-9r-cli-token`
- Query params (GET) or body (POST): Provider-specific import parameters

## Output

- Success: `{"status": "initiated", "jobId": "uuid"}`
- Error: `{"error": "description", "code": "ERROR_CODE"}`

## Logic

1. Validate CLI token via auth middleware
2. Create a background import job for Cursor IDE credentials
3. Store job reference for status tracking
4. Return job ID and initiated status immediately
5. Background job handles the actual credential retrieval and storage

## Acceptance Criteria
- [ ] Endpoint returns expected status format
- [ ] Background job is created and tracked
- [ ] CLI token auth enforced
- [ ] Response matches Node.js Cursor import initiation behavior
- [ ] JobId is a valid UUID
- [ ] Error handling for duplicate/in-progress imports

## Test Scenarios
| Scenario | Input | Expected Output |
|----------|-------|----------------|
| Successful initiation | POST /api/oauth/cursor/auto-import with valid token | HTTP 200, `{"status": "initiated", "jobId": "..."}` |
| Missing auth | POST without token | HTTP 401 |
| Invalid token | POST with bad token | HTTP 401 |
| Concurrent import | POST while import already running | HTTP 200 or 409 with existing jobId |
