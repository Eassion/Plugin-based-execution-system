# Broker IPC Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the plugin execution system to use a centralized Broker with multi-process IPC while preserving existing pipeline, config, dependency, timeout, resilience, and hot reload behavior.

**Architecture:** Broker owns registration, routing, dependency validation, circuit breaking, enablement, plugin metadata, and runtime contexts. Plugins run as child processes exposing UDS JSON services and register themselves through an SDK. Pipeline keeps the existing result shape, but execution uses Broker routing instead of in-process plugin calls.

**Tech Stack:** Go 1.23, Unix domain sockets via `net.UnixConn`, JSON request/response encoding, existing `fsnotify` watcher and Go tests.

---

### Task 1: Broker Registration Core

**Files:**
- Create: `internal/core/dependency_graph.go`
- Create: `internal/core/broker.go`
- Create: `internal/core/broker_test.go`

- [x] Add tests proving Broker dials the submitted UDS path before registration, rejects unreachable plugins, rolls back circular dependencies, and lists registered plugin metadata.
- [x] Implement dependency graph snapshot/rollback and topological cycle validation.
- [x] Implement `Broker.Register` with dial-before-state-change semantics.

### Task 2: Broker Runtime Routing

**Files:**
- Create: `internal/core/circuit_breaker.go`
- Create: `internal/core/ipc.go`
- Modify: `internal/core/broker.go`
- Modify: `internal/core/broker_test.go`

- [x] Add tests for enable/disable without disconnecting, circuit breaker opening after five failures, context timeout rejection, and successful JSON request routing.
- [x] Implement runtime contexts with connection, client, enabled/healthy/draining status, active request count, and breaker.
- [x] Implement `Broker.Invoke`, `SetEnabled`, `ListPlugins`, and old-runtime draining with 10 second forced close.

### Task 3: Pipeline Integration

**Files:**
- Modify: `internal/core/plugin.go`
- Modify: `internal/core/manager.go`
- Modify: `internal/core/pipeline.go`
- Modify: `internal/core/runtime.go`
- Modify: existing core tests

- [x] Add tests proving existing pipeline result semantics remain unchanged when backed by Broker routing.
- [x] Replace in-process `Plugin.Run` execution path with a Broker-backed plugin adapter.
- [x] Keep skipped disabled plugin reporting, per-plugin timeout validation, failure continuation, result duration, and data cloning.

### Task 4: Plugin SDK and Example Workers

**Files:**
- Create: `internal/sdk/server.go`
- Create: `internal/sdk/register.go`
- Create: `cmd/plugin-uppercase/main.go`
- Create: `cmd/plugin-wordcount/main.go`
- Create: `cmd/plugin-timestamp/main.go`
- Modify: `cmd/app/main.go`

- [x] Add SDK tests for JSON request handling.
- [x] Implement UDS JSON plugin server and registration helper.
- [x] Move default example plugins into standalone worker commands.
- [x] Update app bootstrap to start/register example workers for local execution.

### Task 5: Final Verification

**Files:**
- All touched Go files

- [x] Run `gofmt` on modified Go files.
- [x] Run `go test ./...`.
- [x] Run `go run ./cmd/app -input "hello plugin system"` and confirm JSON output preserves current behavior.
