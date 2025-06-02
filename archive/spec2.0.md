# spec.md — Payment Orchestration System (v2.0)

## 🎯 Goals

* Build a high-performance, modular, production-grade payment orchestration platform in Go.
* Support multi-method, multi-step routing with retry, fallback, policy logic, and merchant-specific preferences.
* Absorb messy, real-world merchant inputs via a fault-tolerant external API, while preserving internal contract rigor.
* Incorporate Domain-Driven Design (DDD), Observability, Contract Monitoring, and Schema Versioning via BSR.

---

## 🧱 Architecture Overview

```plaintext
ExternalRequest (chaotic input)
    ↓
Permissive Parser + Validation → Logs + Alerts
    ↓
CoreRequestContext (strict internal type)
    ↓
PlanBuilder → Orchestrator → Router → Processor
    ↓
Observer + ContractMonitor + RetrospectiveReporter
```

---

## 🔁 Dual-Contract Boundary

### ExternalRequest (Permissive Input)

```go
type ExternalRequest struct {
    MerchantID string
    Amount     int
    Method     string
    Currency   string
    Metadata   map[string]string
}
```

### CoreRequestContext (Strict Internal Type)

```go
type CoreRequestContext struct {
    MerchantID       MerchantID
    AmountCents      int
    Currency         Currency
    PaymentMethod    PaymentMethod
    IdempotencyKey   string
    ValidationErrors []ValidationIssue
}
```

* Internal processing always uses `CoreRequestContext`
* Validation errors do not block orchestration, but trigger metrics + alerts

---

## 🧠 Domain Model (DDD)

### Aggregates

* `PaymentPlan`: root aggregate with `[]PaymentStep`, `RemainingAmount`, emitted `Events`

### Domain Events

```go
type StepSucceeded struct {
    StepIndex   int
    ProcessorID string
    LatencyMs   int
    Amount      int
}

type StepFailed struct {
    StepIndex   int
    ProcessorID string
    ErrorCode   ErrorCode
    Retryable   bool
    LatencyMs   int
}

type PaymentCompleted struct {
    TotalAmount int
    RequestID   string
}
```

### Domain Services

* `PlanBuilder`, `CompositePaymentService`, `PaymentPolicyEnforcer`

### Bounded Contexts

* Payments Core
* Plan Assembly (Merchant Config)
* Risk/Fraud Layer (future)
* Lending/Credit (stubbed)
* Observability & Retrospectives

---

## 🧩 Module Decomposition

| Module                  | Responsibility                                             | Interface                                   | Invariants / Contracts                                               |
| ----------------------- | ---------------------------------------------------------- | ------------------------------------------- | -------------------------------------------------------------------- |
| `api`                   | Accepts requests, emits validated `CoreRequestContext`     | `POST /payments`                            | Stateless; logs malformed input; emits alerts                        |
| `planbuilder`           | Builds `PaymentPlan` from merchant config or preferences   | `BuildPlan(RequestContext) → PaymentPlan`   | Deterministic, constraints enforced                                  |
| `orchestrator`          | Executes steps, applies policies, emits `CompositeResult`  | `ExecutePlan(RequestContext, PaymentPlan)`  | Steps must sum to total; stops on failure unless retryable           |
| `router`                | Executes individual `PaymentStep`                          | `Route(RequestContext) → Result`            | Honors timeout, idempotency, and retry policy                        |
| `processor`             | Adapter for external payment APIs (e.g., Stripe, ApplePay) | `Process(ctx, reqCtx) Result`               | Idempotent, panic-proof, normalized error contract                   |
| `observer`              | Metrics, tracing, structured logging                       | `RecordEvent`, `RecordMetric`, `Trace(...)` | Non-blocking, globally tagged                                        |
| `contractmonitor`       | Logs violations of system-level contracts                  | `RecordViolation`, `GenerateReport()`       | Contracts have ID, severity, and ownership                           |
| `retrospectivereporter` | Summarizes system violations + feedback gaps               | Biweekly auto-report                        | Mismatches between system and merchant complaints trigger escalation |

---

## 🔀 Multi-Method Split Payments

```go
type PaymentPlan struct {
    TotalAmountCents int
    Steps            []PaymentStep
}

type PaymentStep struct {
    Method         PaymentMethod
    MaxAmountCents int
    ProcessorID    string
}
```

* Supports fallback routing: e.g. ApplePay (\$60) → Card (\$40)
* Executed sequentially; tracks partial fulfillment

```go
type CompositeResult struct {
    Success         bool
    TotalProcessed  int
    RemainingAmount int
    StepResults     []Result
    FinalError      *ErrorCode
}
```

---

## 🔐 Error Semantics & Retry Contracts

```go
type ErrorCode string

const (
    ErrTimeout         ErrorCode = "timeout"
    ErrDeclined        ErrorCode = "declined"
    ErrInvalidCard     ErrorCode = "invalid_card"
    ErrNetworkFailure  ErrorCode = "network_failure"
    ErrUnknown         ErrorCode = "unknown"
)

type Result struct {
    Success          bool
    ProcessorID      string
    ErrorCode        ErrorCode
    AmountProcessed  int
    LatencyMs        int
    Retryable        bool
}
```

---

## 🧪 Testing Strategy

| Layer          | Strategy                                  |
| -------------- | ----------------------------------------- |
| Unit Tests     | Validate each module in isolation         |
| Integration    | Full E2E tests: API → CompositeResult     |
| Property-Based | Validate idempotency, consistency, limits |
| Retrospectives | Simulate complaints vs. internal logs     |

---

## 📊 Observability + Contract Monitoring

* All steps emit domain events, metrics, and span traces
* Violations emitted as:

```go
type ContractViolation struct {
    ContractID string
    RequestID  string
    Severity   string
    Message    string
}
```

* Retrospective reports every 2 weeks:

```yaml
violations:
  - id: router/timeout
    count: 932
    external_reports: 8
    mismatch: false
  - id: plan/coverage
    count: 41
    external_reports: 17
    mismatch: true
```

---

## 📦 Schema Versioning + Buf Registry

| Component           | Format   | BSR Package             |
| ------------------- | -------- | ----------------------- |
| `PaymentPlan`       | Protobuf | `payments.plan.v1`      |
| `CompositeResult`   | JSON     | `payments.result.v1`    |
| `ContractViolation` | JSON     | `payments.contracts.v1` |
| `Domain Events`     | Protobuf | `payments.events.v1`    |
| `Merchant Config`   | Protobuf | `payments.config.v1`    |

All changes are subject to `buf lint` + `buf breaking` checks.

---

## 🧨 Blast Radius Summary

| Component             | Example Change                                   | Impact |
| --------------------- | ------------------------------------------------ | ------ |
| `PaymentPlan`         | Switch to DAG, conditional logic                 | 🟥     |
| `Processor Interface` | Add streaming, async response                    | 🟧     |
| `Result` Struct       | Remove `Retryable`, break backward compatibility | 🟧     |
| `Observer`            | Add new metrics                                  | 🟩     |
| `PlanBuilder`         | Add per-region routing policies                  | 🟨     |

---

## ✅ Summary of Guarantees

| Capability                  | Mechanism                                                          |
| --------------------------- | ------------------------------------------------------------------ |
| Internal consistency        | `CoreRequestContext`, `PaymentPlan` as strictly enforced contracts |
| Fault tolerance at boundary | Dual-contract model with validation + continuation                 |
| Modular extensibility       | Strict layering, adapters, config-driven routing                   |
| Observability + alerting    | Metrics, traces, domain events + ContractMonitor                   |
| Feedback loop readiness     | Biweekly retrospectives, escalation on contract mismatch           |
| Schema safety               | BSR + semantic versioning + CI breaking change enforcement         |

---

**Status**: Finalized for external review. DDD-aligned. Contract-safe. Observability-native. Resilient by design.
