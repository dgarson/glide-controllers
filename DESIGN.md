# Streamline Controller Framework

> **Product Requirements Document & Technical Specification**

| Field       | Value                              |
|-------------|------------------------------------|
| **Version** | 0.1 (POC Phase)                    |
| **Status**  | Draft                              |
| **Target**  | Kubernetes Operator Developers     |

---

## Table of Contents

- [1. Executive Summary](#1-executive-summary)
  - [1.1 The Problem](#11-the-problem)
  - [1.2 The Solution](#12-the-solution)
- [2. User Experience & Interfaces](#2-user-experience--interfaces)
  - [2.1 The Developer Contract](#21-the-developer-contract)
- [3. Technical Architecture](#3-technical-architecture)
  - [3.1 Layer 1: The Registration Facade](#31-layer-1-the-registration-facade)
  - [3.2 Layer 2: The Generic Engine (Middleware)](#32-layer-2-the-generic-engine-middleware)
  - [3.3 Layer 3: The Context Wrapper](#33-layer-3-the-context-wrapper)
- [4. API Specification (Go)](#4-api-specification-go)
  - [4.1 The Handler Interface](#41-the-handler-interface)
  - [4.2 The Context](#42-the-context)
  - [4.3 The Result Object](#43-the-result-object)
- [5. Detailed Component Behavior (The "Engine")](#5-detailed-component-behavior-the-engine)
  - [5.1 Finalizer Management Logic](#51-finalizer-management-logic)
  - [5.2 Smart Status Patching](#52-smart-status-patching)
- [6. Implementation Roadmap (POC)](#6-implementation-roadmap-poc)
- [7. Future Considerations (Post-POC)](#7-future-considerations-post-poc)

---

## 1. Executive Summary

**Streamline** is a high-level, opinionated framework built on top of Kubernetes `controller-runtime`. It leverages Go Generics (1.20+) to abstract away the repetitive machinery of Kubernetes controllers (loops, cache fetches, finalizers, patch management).

### Core Philosophy

> Move developers from thinking about **"Event Loops"** (low-level plumbing) to thinking about **"State Transitions"** (business logic).

### 1.1 The Problem

The `controller-runtime` library is powerful but verbose. A standard controller requires the developer to manually:

1. Fetch the resource from the cache
2. Handle `IsNotFound` errors
3. Manage the lifecycle of Finalizers (add string, update, remove string, update)
4. Calculate Status patches (`DeepCopy`, `Modify`, `MergeFrom`)
5. Setup Event Recorders and Contexts

### 1.2 The Solution

Streamline provides a `GenericReconciler[T]` that handles steps 1-5 automatically. The developer simply implements a `Sync` method for their specific Type.

---

## 2. User Experience & Interfaces

### 2.1 The Developer Contract

The developer should interact only with **strongly typed objects**. They should not need to import `client` or `reconcile` packages directly in their business logic.

#### The Old Way (Standard Controller-Runtime)

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &MyObj{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Manual Finalizer Logic...
    // Manual Status Patching Logic...

    return ctrl.Result{}, nil
}
```

#### The Streamline Way

```go
type MyHandler struct{}

func (h *MyHandler) Sync(ctx context.Context, obj *MyObj, sCtx *streamline.Context) (streamline.Result, error) {
    // Logic only. Object is already fetched.
    if obj.Spec.Size < 0 {
        sCtx.Event.Warning("InvalidConfig", "Size cannot be negative")
        obj.Status.State = "Error"
        return streamline.Stop(), nil
    }

    obj.Status.State = "Running"
    return streamline.RequeueAfter(10 * time.Minute), nil
}
```

---

## 3. Technical Architecture

The architecture consists of **three layers**:

```
┌─────────────────────────────────────────────────────────────┐
│                 Layer 1: Registration Facade                │
│            (Builder pattern, Manager integration)           │
├─────────────────────────────────────────────────────────────┤
│              Layer 2: Generic Engine (Middleware)           │
│     (Fetch, Snapshot, Lifecycle Routing, Smart Patching)    │
├─────────────────────────────────────────────────────────────┤
│                 Layer 3: Context Wrapper                    │
│              (Logging, Events, Client Access)               │
└─────────────────────────────────────────────────────────────┘
```

### 3.1 Layer 1: The Registration Facade

A **Builder pattern** that bridges the `controller-runtime` Manager with our Generic Engine.

**Responsibilities:**

- Accepts a typed CRD struct (e.g., `&CronJob{}`)
- Accepts a Handler implementation
- Configures standard predicates (generation changes) by default
- Wires up the underlying `controller-runtime` Controller

### 3.2 Layer 2: The Generic Engine (Middleware)

This is the **core of the framework**. It implements `reconcile.Reconciler`.

**Workflow per Request:**

1. **Instantiation:** Create a new instance of `T` using reflection
2. **Fetch:** `client.Get`. If not found, return `nil` (assume deleted)
3. **Snapshot:** `obj.DeepCopy()` to store the "Original" state for patch calculation
4. **Lifecycle Routing:**
   - If `DeletionTimestamp` is set → Call `Handler.Finalize()`
   - If `DeletionTimestamp` is nil → Call `Handler.Sync()`
5. **Smart Patching:** Compare "Current" state vs "Original" state. If different, execute `client.Status().Patch()`
6. **Result Translation:** Convert `streamline.Result` to `ctrl.Result`

### 3.3 Layer 3: The Context Wrapper

A helper struct passed to handlers to simplify auxiliary actions.

**Responsibilities:**

| Component       | Description                                                                 |
|-----------------|-----------------------------------------------------------------------------|
| **Logging**     | Wraps `logr.Logger` with pre-filled namespaced/name keys                   |
| **Events**      | Wraps `record.EventRecorder` to emit events without passing object ref     |
| **Client Access** | Provides raw client access only if absolutely necessary                  |

---

## 4. API Specification (Go)

### 4.1 The Handler Interface

```go
package streamline

// Handler defines the business logic for a resource T.
type Handler[T client.Object] interface {
    // Sync is called for creation and updates.
    // The object is guaranteed to exist and not be deleting.
    Sync(ctx context.Context, obj T, sCtx *Context) (Result, error)
}

// FinalizingHandler is an optional interface.
// Implement this if your resource needs cleanup logic.
type FinalizingHandler[T client.Object] interface {
    Handler[T]
    // Finalize is called when the object is marked for deletion.
    // Return Result.Done() to confirm cleanup is finished and the finalizer can be removed.
    Finalize(ctx context.Context, obj T, sCtx *Context) (Result, error)
}
```

### 4.2 The Context

```go
type Context struct {
    Client client.Client
    Log    logr.Logger
    Event  EventRecorder
}

type EventRecorder interface {
    Normal(reason, message string)
    Warning(reason, message string)
}
```

### 4.3 The Result Object

```go
type Result struct {
    Requeue      bool
    RequeueAfter time.Duration
    Err          error
}

// Helper constructors for fluency
func Stop() Result { ... }
func Requeue() Result { ... }
func RequeueAfter(d time.Duration) Result { ... }
func Error(err error) Result { ... }
```

---

## 5. Detailed Component Behavior (The "Engine")

### 5.1 Finalizer Management Logic

The Engine must handle the complex "dance" of finalizers automatically.

#### Algorithm

```
┌─────────────────────────────────────────────────────────────┐
│              Check: DeletionTimestamp set?                  │
└─────────────────────┬───────────────────────────────────────┘
                      │
         ┌────────────┴────────────┐
         │                         │
    ┌────▼────┐              ┌─────▼─────┐
    │   YES   │              │    NO     │
    │(Deleting)│              │(Not Del.) │
    └────┬────┘              └─────┬─────┘
         │                         │
         ▼                         ▼
  Implements                 Implements
  FinalizingHandler?         FinalizingHandler?
         │                         │
    ┌────┴────┐              ┌─────┴─────┐
   YES       NO             YES         NO
    │         │               │           │
    ▼         ▼               ▼           ▼
 Call      Remove          Add        Proceed
 Finalize  finalizer      finalizer   to Sync()
    │      & return       if missing
    │                         │
    ▼                         ▼
 Stop()?─────────────► Remove finalizer
    │                   Update()
   NO
    │
    ▼
 Return result
 (keep finalizer)
```

**Detailed Steps:**

1. Check if `obj` has `DeletionTimestamp` set
2. **IF YES (Deleting):**
   - Does the Handler implement `FinalizingHandler`?
     - **NO:** Remove our finalizer string (if present) and return
     - **YES:** Call `Handler.Finalize()`
       - If it returns `Stop()` (Success) → Remove finalizer string → `Update()`
       - If it returns `Error`/`Requeue` → Do NOT remove finalizer → Return result
3. **IF NO (Not Deleting):**
   - Does Handler implement `FinalizingHandler`?
     - **YES:** Check if finalizer string exists on obj
       - If missing → Add finalizer string → `Patch()` → Requeue immediately
     - **NO:** Proceed to `Sync()`

### 5.2 Smart Status Patching

To prevent `"Object has been modified"` errors and reduce API load, we use **Server-Side Apply** or **MergePatch**.

#### Algorithm

```go
// 1. Snapshot before handler execution
original := obj.DeepCopyObject().(client.Object)

// 2. Run handler (modifies obj in place)
result := Handler.Sync(..., obj, ...)

// 3. Calculate and apply patch
patch := client.MergeFrom(original)
err := client.Status().Patch(ctx, obj, patch)
```

> **Note:** The Engine captures the error from `Patch`. If `Patch` fails, the Reconcile fails.

---

## 6. Implementation Roadmap (POC)

| Phase | Name           | Deliverables                                                    |
|-------|----------------|-----------------------------------------------------------------|
| 1     | The Skeleton   | Create `pkg/streamline`, define `Handler[T]`, `Result`, `Context` |
| 2     | The Engine     | Implement `GenericReconciler[T]`, Fetch logic, Smart Patching   |
| 3     | Lifecycle      | Finalizer detection, Add/Remove finalizer loops                 |
| 4     | Validation     | Sample `examples/guestbook`, compare LoC and readability        |

---

## 7. Future Considerations (Post-POC)

| Feature            | Description                                                                                       |
|--------------------|---------------------------------------------------------------------------------------------------|
| **Metrics**        | Auto-inject Prometheus metrics for Sync duration and Error rates labeled by Controller Name      |
| **Testing Framework** | `streamlinetest` package for unit testing `Handler.Sync` without a real K8s client using mock Contexts |
| **Webhooks**       | Abstraction for Admission Webhooks using the same Handler style (e.g., `Validate(ctx, obj)`)     |
