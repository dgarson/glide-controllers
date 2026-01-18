# Streamline Controller Framework - Task Breakdown

> **Project Implementation Plan**

| Field           | Value                              |
|-----------------|------------------------------------|
| **Version**     | 0.1 (POC Phase)                    |
| **Last Updated**| 2026-01-17                         |
| **Status**      | Planning                           |

---

## Table of Contents

- [Legend](#legend)
- [Dependency Overview](#dependency-overview)
- [Phase 1: The Skeleton](#phase-1-the-skeleton)
- [Phase 2: The Engine](#phase-2-the-engine)
- [Phase 3: Lifecycle Management](#phase-3-lifecycle-management)
- [Phase 4: Validation](#phase-4-validation)
- [Workstream Analysis](#workstream-analysis)

---

## Legend

| Symbol | Meaning |
|--------|---------|
| 🔴 | **Blocking** - Must complete before dependent tasks can start |
| 🟢 | **Parallelizable** - Can be worked on concurrently with other tasks |
| 🟡 | **Partially Blocking** - Blocks some tasks but not all |
| `[P1.1]` | Task reference ID for dependency tracking |

---

## Dependency Overview

```
Phase 1 (Skeleton)          Phase 2 (Engine)           Phase 3 (Lifecycle)      Phase 4 (Validation)
─────────────────────────────────────────────────────────────────────────────────────────────────────

[P1.1] pkg/streamline ──────┬──► [P2.1] GenericReconciler ──► [P3.1] Finalizer Detection
       (BLOCKING)           │           (BLOCKING)                   (BLOCKING)
                            │                                             │
[P1.2] Handler[T] ──────────┼──► [P2.2] Fetch Logic ─────────► [P3.2] Add Finalizer Loop
       (BLOCKING)           │           (BLOCKING)                        │
                            │                                             │
[P1.3] Result Type ─────────┤                                 [P3.3] Remove Finalizer Loop
       (BLOCKING)           │                                             │
                            │                                             ▼
[P1.4] Context Wrapper ─────┴──► [P2.3] Smart Patching ──────► [P4.1-4.4] Validation
       (BLOCKING)                       (BLOCKING)              (PARALLELIZABLE)
```

---

## Phase 1: The Skeleton

> **Goal:** Establish the foundational package structure and core type definitions.

| ID | Task | Status | Dependency |
|----|------|--------|------------|
| P1 | **Phase 1: The Skeleton** | ⬜ Not Started | None |

### P1.1 - Create Package Structure 🔴

**Description:** Initialize the `pkg/streamline` package with proper Go module structure.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P1.1.1 | Create `pkg/streamline/` directory | ⬜ | — |
| P1.1.2 | Create `pkg/streamline/doc.go` with package documentation | ⬜ | — |
| P1.1.3 | Verify Go module includes new package | ⬜ | — |

**Blocks:** All subsequent tasks

---

### P1.2 - Define Handler Interface 🔴

**Description:** Define the generic `Handler[T]` and `FinalizingHandler[T]` interfaces.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P1.2.1 | Create `pkg/streamline/handler.go` | ⬜ | — |
| P1.2.2 | Define `Handler[T client.Object]` interface with `Sync()` method | ⬜ | — |
| P1.2.3 | Define `FinalizingHandler[T client.Object]` interface with `Finalize()` method | ⬜ | — |
| P1.2.4 | Add GoDoc comments for interface contracts | ⬜ | — |

**Blocks:** P2.1, P2.2, P3.1, P3.2, P3.3

---

### P1.3 - Define Result Type 🔴

**Description:** Create the `Result` struct and helper constructor functions.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P1.3.1 | Create `pkg/streamline/result.go` | ⬜ | — |
| P1.3.2 | Define `Result` struct with `Requeue`, `RequeueAfter`, `Err` fields | ⬜ | — |
| P1.3.3 | Implement `Stop()` constructor | ⬜ | — |
| P1.3.4 | Implement `Requeue()` constructor | ⬜ | — |
| P1.3.5 | Implement `RequeueAfter(duration)` constructor | ⬜ | — |
| P1.3.6 | Implement `Error(err)` constructor | ⬜ | — |
| P1.3.7 | Add unit tests for Result constructors | ⬜ | 🟢 Can parallelize |

**Blocks:** P2.1, P2.3

---

### P1.4 - Create Context Wrapper 🔴

**Description:** Implement the `Context` struct that wraps logging, events, and client access.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P1.4.1 | Create `pkg/streamline/context.go` | ⬜ | — |
| P1.4.2 | Define `Context` struct with `Client`, `Log`, `Event` fields | ⬜ | — |
| P1.4.3 | Define `EventRecorder` interface with `Normal()` and `Warning()` methods | ⬜ | — |
| P1.4.4 | Implement `EventRecorder` wrapper around `record.EventRecorder` | ⬜ | — |
| P1.4.5 | Implement logger wrapper with pre-filled namespace/name keys | ⬜ | — |
| P1.4.6 | Add unit tests for Context and EventRecorder | ⬜ | 🟢 Can parallelize |

**Blocks:** P2.1, P2.2

---

## Phase 2: The Engine

> **Goal:** Implement the core reconciliation engine that handles fetch, snapshot, and patching.

| ID | Task | Status | Dependency |
|----|------|--------|------------|
| P2 | **Phase 2: The Engine** | ⬜ Not Started | Phase 1 Complete |

### P2.1 - Implement GenericReconciler[T] 🔴

**Description:** Create the generic reconciler struct that implements `reconcile.Reconciler`.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P2.1.1 | Create `pkg/streamline/reconciler.go` | ⬜ | — |
| P2.1.2 | Define `GenericReconciler[T client.Object]` struct | ⬜ | — |
| P2.1.3 | Add fields: `Client`, `Handler`, `Scheme`, `EventRecorder` | ⬜ | — |
| P2.1.4 | Implement `Reconcile(ctx, req)` method signature | ⬜ | — |
| P2.1.5 | Implement type instantiation using reflection (`new(T)`) | ⬜ | — |

**Depends on:** P1.1, P1.2, P1.3, P1.4
**Blocks:** P2.2, P2.3, P3.1

---

### P2.2 - Implement Fetch Logic 🔴

**Description:** Add the object fetch and "not found" handling logic to the reconciler.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P2.2.1 | Implement `client.Get()` call in Reconcile method | ⬜ | — |
| P2.2.2 | Handle `IsNotFound` error - return nil (assume deleted) | ⬜ | — |
| P2.2.3 | Handle other errors - return error with requeue | ⬜ | — |
| P2.2.4 | Add logging for fetch operations | ⬜ | — |
| P2.2.5 | Add unit tests for fetch scenarios | ⬜ | 🟢 Can parallelize |

**Depends on:** P2.1, P1.4
**Blocks:** P2.3, P3.1

---

### P2.3 - Implement Smart Patching 🔴

**Description:** Implement the snapshot and merge-patch logic for status updates.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P2.3.1 | Implement `obj.DeepCopy()` snapshot before handler execution | ⬜ | — |
| P2.3.2 | Call handler's `Sync()` method with object and context | ⬜ | — |
| P2.3.3 | Implement `client.MergeFrom(original)` patch calculation | ⬜ | — |
| P2.3.4 | Implement `client.Status().Patch()` execution | ⬜ | — |
| P2.3.5 | Handle patch errors appropriately | ⬜ | — |
| P2.3.6 | Implement result translation from `streamline.Result` to `ctrl.Result` | ⬜ | — |
| P2.3.7 | Add unit tests for patching scenarios | ⬜ | 🟢 Can parallelize |

**Depends on:** P2.1, P2.2, P1.3
**Blocks:** P3.1, P3.2, P3.3

---

## Phase 3: Lifecycle Management

> **Goal:** Implement automatic finalizer management for resource cleanup.

| ID | Task | Status | Dependency |
|----|------|--------|------------|
| P3 | **Phase 3: Lifecycle Management** | ⬜ Not Started | Phase 2 Complete |

### P3.1 - Implement Finalizer Detection Logic 🔴

**Description:** Add logic to detect deletion state and check for `FinalizingHandler` implementation.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P3.1.1 | Define finalizer string constant (e.g., `streamline.io/finalizer`) | ⬜ | — |
| P3.1.2 | Implement `DeletionTimestamp` check | ⬜ | — |
| P3.1.3 | Implement type assertion for `FinalizingHandler` interface | ⬜ | — |
| P3.1.4 | Add routing logic: deletion path vs. sync path | ⬜ | — |
| P3.1.5 | Add unit tests for detection logic | ⬜ | 🟢 Can parallelize |

**Depends on:** P2.1, P2.2, P2.3
**Blocks:** P3.2, P3.3

---

### P3.2 - Implement "Add Finalizer" Loop 🟢

**Description:** Implement logic to add finalizer string when handler implements `FinalizingHandler`.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P3.2.1 | Check if finalizer string exists on object | ⬜ | — |
| P3.2.2 | If missing, add finalizer to `obj.Finalizers` | ⬜ | — |
| P3.2.3 | Patch object to persist finalizer | ⬜ | — |
| P3.2.4 | Requeue immediately after adding finalizer | ⬜ | — |
| P3.2.5 | Add unit tests for add finalizer flow | ⬜ | 🟢 Can parallelize |

**Depends on:** P3.1
**Can parallelize with:** P3.3

---

### P3.3 - Implement "Remove Finalizer" Loop 🟢

**Description:** Implement logic to remove finalizer after successful finalization.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P3.3.1 | Call `Handler.Finalize()` when deletion detected | ⬜ | — |
| P3.3.2 | Check if result is `Stop()` (success) | ⬜ | — |
| P3.3.3 | If success, remove finalizer string from `obj.Finalizers` | ⬜ | — |
| P3.3.4 | Update object to persist finalizer removal | ⬜ | — |
| P3.3.5 | If not success, preserve finalizer and return result | ⬜ | — |
| P3.3.6 | Handle case where handler does NOT implement `FinalizingHandler` | ⬜ | — |
| P3.3.7 | Add unit tests for remove finalizer flow | ⬜ | 🟢 Can parallelize |

**Depends on:** P3.1
**Can parallelize with:** P3.2

---

## Phase 4: Validation

> **Goal:** Validate the framework with a sample implementation and compare with standard approach.

| ID | Task | Status | Dependency |
|----|------|--------|------------|
| P4 | **Phase 4: Validation** | ⬜ Not Started | Phase 3 Complete |

### P4.1 - Create Sample Project Structure 🟢

**Description:** Set up the `examples/guestbook` sample project.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P4.1.1 | Create `examples/guestbook/` directory | ⬜ | — |
| P4.1.2 | Define Guestbook CRD spec and status | ⬜ | — |
| P4.1.3 | Generate CRD manifests | ⬜ | — |
| P4.1.4 | Set up basic main.go with manager | ⬜ | — |

**Depends on:** Phase 3 Complete
**Can parallelize with:** P4.2

---

### P4.2 - Implement Standard Controller-Runtime Controller 🟢

**Description:** Create a traditional controller-runtime implementation for comparison.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P4.2.1 | Create `examples/guestbook/controllers/standard/` | ⬜ | — |
| P4.2.2 | Implement standard `Reconcile()` with manual fetch | ⬜ | — |
| P4.2.3 | Implement manual finalizer logic | ⬜ | — |
| P4.2.4 | Implement manual status patching | ⬜ | — |
| P4.2.5 | Count lines of code | ⬜ | — |

**Depends on:** P4.1
**Can parallelize with:** P4.3

---

### P4.3 - Implement Streamline Controller 🟢

**Description:** Create a Streamline-based implementation for comparison.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P4.3.1 | Create `examples/guestbook/controllers/streamline/` | ⬜ | — |
| P4.3.2 | Implement `Handler[*Guestbook]` with `Sync()` method | ⬜ | — |
| P4.3.3 | Implement `FinalizingHandler` with `Finalize()` method | ⬜ | — |
| P4.3.4 | Wire up with `GenericReconciler` | ⬜ | — |
| P4.3.5 | Count lines of code | ⬜ | — |

**Depends on:** P4.1, Phase 3 Complete
**Can parallelize with:** P4.2

---

### P4.4 - Compare and Document Results 🟢

**Description:** Document the comparison between approaches.

| ID | Subtask | Status | Notes |
|----|---------|--------|-------|
| P4.4.1 | Compare Lines of Code (LoC) between implementations | ⬜ | — |
| P4.4.2 | Assess readability differences | ⬜ | — |
| P4.4.3 | Document boilerplate reduction metrics | ⬜ | — |
| P4.4.4 | Update DESIGN.md with validation results | ⬜ | — |

**Depends on:** P4.2, P4.3

---

## Workstream Analysis

### Blocking Dependencies (Critical Path)

The following tasks are on the **critical path** and must be completed sequentially:

```
P1.1 → P1.2 → P2.1 → P2.2 → P2.3 → P3.1 → P3.2/P3.3 → P4.x
 │      │      │
 │      │      └─ P1.3 (can parallel with P1.2)
 │      │
 │      └─ P1.4 (can parallel with P1.2, P1.3)
 │
 └─ Single blocking root
```

### Parallelizable Workstreams

| Workstream | Tasks | Notes |
|------------|-------|-------|
| **Core Types** | P1.2, P1.3, P1.4 | Can develop in parallel after P1.1 |
| **Unit Tests** | P1.3.7, P1.4.6, P2.2.5, P2.3.7, P3.1.5, P3.2.5, P3.3.7 | Can be written alongside implementation |
| **Finalizer Loops** | P3.2, P3.3 | Can develop in parallel after P3.1 |
| **Validation Examples** | P4.1, P4.2, P4.3 | Can largely be parallelized |

### Recommended Team Allocation

| Developer | Primary Focus | Secondary Focus |
|-----------|---------------|-----------------|
| **Dev 1** | P1.1 → P2.1 → P2.2 → P3.1 (Critical Path) | — |
| **Dev 2** | P1.2, P1.3, P1.4 (Parallel Core Types) | Unit Tests |
| **Dev 3** | P2.3, P3.2, P3.3 (Once unblocked) | P4.x Validation |

### Risk Assessment

| Risk | Mitigation |
|------|------------|
| **Generic type complexity** | Prototype reflection-based instantiation early (P2.1.5) |
| **Patch conflicts** | Add comprehensive tests for concurrent modification scenarios |
| **Finalizer edge cases** | Test deletion races and multiple finalizer scenarios |

---

## Progress Tracking

| Phase | Total Tasks | Completed | Progress |
|-------|-------------|-----------|----------|
| Phase 1 | 4 | 0 | 0% |
| Phase 2 | 3 | 0 | 0% |
| Phase 3 | 3 | 0 | 0% |
| Phase 4 | 4 | 0 | 0% |
| **Total** | **14** | **0** | **0%** |
