Here’s your fully updated **`spec.md` (v1.4)** — now incorporating:

* 🧱 Strict internal data contracts with fault-tolerant boundaries
* 📦 Buf Schema Registry (BSR) support with package ownership
* 🔐 Dual-contract model for external permissiveness + internal invariants
* 🔁 Merchant feedback loops for invalid/malformed input without silent drops
* ✅ Reinforced observability and DDD-aligned layering

---

````markdown
# spec.md — Payment Orchestration System (v1.4)

## 🧭 Goals

- Build a modular, production-grade payment orchestration system in Go.
- Support multi-step routing, multiple processors, fallback strategies.
- Handle real-world input chaos from merchants and partners without compromising internal integrity.
- Embed Domain-Driven Design, Observability, and Contract Monitoring from day one.
- Use Protobuf + Buf Schema Registry (BSR) to version and validate all data contracts.

---

## 🔁 Architecture Overview

```plaintext
External API Input
   ↓
Permissive Parsing + Validation Reporting
   ↓
CoreRequestContext (strict internal type)
   ↓
PlanBuilder → Orchestrator → Router → Processor
   ↓
Observer · ContractMonitor · Retrospective Reporter
````

---

## 🧱 Dual-Contract Design (Fault-Tolerant Core)

### External API (Permissive)

```go
type ExternalRequest struct {
    MerchantID string
    Amount     int
    Method     string
    Currency   string
    Metadata   map[string]string
}
```

### Internal Core Representation (Strict)

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

* Validation errors are tracked and surfaced, but do not block orchestration.
* System continues defensively with fallback/defaults.
* Alerts and metrics emitted if fields are malformed, unknown, or missing.

---

## 📦 Buf Schema Registry (BSR) Integration

### Protobuf Packages

| Domain Area         | Package Name            |
| ------------------- | ----------------------- |
| Payment Plan        | `payments.plan.v1`      |
| Composite Result    | `payments.result.v1`    |
| Contract Violations | `payments.contracts.v1` |
| Domain Events       | `payments.events.v1`    |
| Merchant Config     | `payments.config.v1`    |

* All packages registered with Buf Schema Registry.
* `buf.yaml` configured with lint + breaking check policies.
* Semantic versioning used for package evolution (`v1`, `v2beta`, etc.)
* Every serialized payload includes `schema_version`.

---

## 🧠 DDD Foundation

### Aggregates

* `PaymentPlan`: root aggregate for step-based execution, event emission, contract validation.

### Domain Events

* `StepSucceeded`, `StepFailed`, `PaymentCompleted`, all with stable schema versions.

### Domain Services

* `PlanBuilder`, `CompositePaymentService`, `PaymentPolicyEnforcer`

### Bounded Contexts

| Context              | Modules                             |
| -------------------- | ----------------------------------- |
| Payment Routing      | Orchestrator, Router, Processor     |
| Plan Assembly        | PlanBuilder, Config, Merchants      |
| Policy Enforcement   | Retry logic, fraud constraints      |
| Feedback + Contracts | Observer, ContractMonitor, Reporter |

---

## 🔍 Observability

* `Observer` emits metrics, logs, trace spans, and domain events.
* Events include structured tags (`merchant_id`, `step_index`, `error_code`, etc.)
* Observability hooks embedded in every orchestrated step.

---

## 📊 Contract Monitoring & Retrospectives

* System-level contracts defined with IDs (e.g., `router/timeout`, `plan/coverage`)
* `ContractViolation` emitted when invariant is violated
* `RetrospectiveReport` generated every 2 weeks with:

```yaml
violations:
  - id: plan/coverage
    count: 41
    external_reports: 17
    mismatch: true
```

* Triggers Slack/email/ticket escalation if internal violations diverge from merchant complaints

---

## 🧪 Testing Strategy

* Validate that all malformed requests are still routed as `CoreRequestContext` with `ValidationErrors`
* Regression tests for each schema in BSR
* Contract tests for serialized domain events
* Retrospective test suite: simulated replay, alert simulation, mismatch handling

---

## ✅ Summary: Architectural Guarantees

| Guarantee                        | How It's Achieved                                       |
| -------------------------------- | ------------------------------------------------------- |
| Core system is stable and strict | Dual-contract model (`ExternalRequest` → `CoreContext`) |
| External chaos is tolerated      | Permissive parser, validation buffer, fallback routing  |
| Violations never go silent       | Observability + ContractMonitor pipeline                |
| Data contracts are versioned     | Buf Schema Registry with lint + breaking checks         |
| Internal feedback loops run      | Biweekly retrospectives with system/user mismatch diff  |

---

## 📦 Versioning & Storage

| Component           | Format       | Schema Policy                     |
| ------------------- | ------------ | --------------------------------- |
| `PaymentPlan`       | Protobuf     | Versioned via BSR (`plan.v1`)     |
| `CompositeResult`   | JSON         | Additive fields only              |
| `Domain Events`     | Protobuf     | Version field embedded            |
| `ContractViolation` | JSON / DB    | Stable schema + contract registry |
| `IdempotencyStore`  | Hash + state | Input hash is frozen              |

---

**Status**: Complete, version-controlled, BSR-backed, fault-tolerant spec.


Here is the **updated `spec.md` (v1.2)** with full support for **merchant-specific default routing plans**, including the new `PlanBuilder` module and all necessary architectural integrations.

---

````markdown
# spec.md — Payment Orchestration System (v1.2)

## 🧭 Goals

- High-performance, modular, latency-bound (<100ms) payment orchestration system in Go.
- Production-grade from the start, no throwaway code.
- Interfaces defined via Protobuf for future language-neutral clients.
- Modular enough to support:
  - New payment methods (e.g. Apple Pay, Bank Transfers)
  - Multi-step composite payments (e.g. Apple Pay + Credit Card fallback)
  - Merchant-specific routing preferences (e.g. default to co-branded cards)

---

## 🧱 System Overview

### Full Request Flow (with PlanBuilder)

```mermaid
sequenceDiagram
    participant M as Merchant
    participant API as API Layer
    participant Builder as PlanBuilder
    participant Orchestrator as Payment Orchestrator
    participant Router as Payment Router
    participant Processors as Payment Processors
    participant Logger as Logger & Metrics

    M->>API: POST /payments
    API->>Builder: BuildPlan(MerchantID, ReqContext)
    Builder-->>API: PaymentPlan
    API->>Orchestrator: ExecutePlan(RequestContext, Plan)
    Orchestrator->>Router: Route each step
    Router->>Processors: Execute via processor adapters
    Processors-->>Router: Step Results
    Router-->>Orchestrator: Aggregated Step Results
    Orchestrator->>Logger: CompositeResult
    Orchestrator-->>API: Final response
````

---

## 🔧 Key Data Types (Expanded)

### `PaymentPlan`, `PaymentStep`

```go
type PaymentPlan struct {
    TotalAmountCents int
    Steps            []PaymentStep
}

type PaymentStep struct {
    Method          PaymentMethod
    MaxAmountCents  int
    ProcessorID     string
}
```

### `CompositeResult`

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

## 🧩 Module Table (Expanded)

| Module         | Responsibility                                        | Interface                                        | Invariants                                                   |
| -------------- | ----------------------------------------------------- | ------------------------------------------------ | ------------------------------------------------------------ |
| `api`          | Accepts requests, constructs `RequestContext`         | `POST /payments`                                 | Validates inputs, calls builder + orchestrator               |
| `planbuilder`  | Constructs dynamic `PaymentPlan` from merchant config | `BuildPlan(RequestContext) PaymentPlan`          | Plan must cover total amount, honor merchant constraints     |
| `orchestrator` | Executes multi-step plans                             | `ExecutePlan(ctx, reqCtx, plan) CompositeResult` | Executes steps in order; aggregates results; enforces limits |
| `router`       | Executes a single step                                | `Route(RequestContext) Result`                   | Honors `MaxAmountCents`; returns fast or retryable errors    |
| `processor`    | Processor abstraction layer                           | `Process(ctx, reqCtx) Result`                    | Implements timeout guards, returns normalized error codes    |
| `config`       | Returns merchant-specific config                      | `GetConfig(merchantID)`                          | Always cached; must include preferred processors             |

---

## 🧠 New Module: `PlanBuilder`

### Responsibility

* Generates merchant-specific `PaymentPlan` for each payment
* Encapsulates fallback priorities and method ordering

### Interface

```go
type PlanBuilder interface {
    BuildPlan(ctx context.Context, merchantID string, req RequestContext) (PaymentPlan, error)
}
```

### Sample Output

```go
plan := PaymentPlan{
    TotalAmountCents: 10000,
    Steps: []PaymentStep{
        { Method: "CARD", ProcessorID: "cobranded_card", MaxAmountCents: 10000 },
        { Method: "CARD", ProcessorID: "stripe",         MaxAmountCents: 10000 },
    },
}
```

### Invariants

* Sum of `MaxAmountCents` ≥ `TotalAmountCents`
* Processor order reflects merchant preferences
* Returned plan is deterministic for the same input

---

## ⚙️ Protobuf Schema Update (Sketch)

```protobuf
enum PaymentMethod {
    CARD = 0;
    APPLE_PAY = 1;
    BANK_TRANSFER = 2;
}

message PaymentStep {
    PaymentMethod method = 1;
    int32 max_amount_cents = 2;
    string processor_id = 3;
}

message PaymentPlan {
    int32 total_amount_cents = 1;
    repeated PaymentStep steps = 2;
}
```

---

## 🧪 Testing Strategy (Extended)

### PlanBuilder Tests

* Returns correct steps based on merchant config
* Rejects plans with insufficient coverage
* Handles malformed merchant configurations gracefully

### Integration Tests

* Confirm co-branded fallback order is respected
* Ensure Stripe is only invoked if cobranded card fails
* Validate latency/performance with dynamic plans

---

## 🧠 Summary of Benefits

| Benefit                       | Achieved Through                                                |
| ----------------------------- | --------------------------------------------------------------- |
| Merchant-specific defaults    | `PlanBuilder` module, config-driven                             |
| Dynamic routing logic         | Decoupled from `router` or `api`                                |
| Predictable fallback behavior | Per-step priority ordering, idempotent logic                    |
| Modular plan generation       | Swappable `PlanBuilder` implementation (static, ML-based, etc.) |

---

## ✅ Final Layered View

```plaintext
API Layer
│
├──→ PlanBuilder (creates PaymentPlan based on merchant config)
│
├──→ Orchestrator (executes steps in sequence)
│     ├──→ Router (routes individual steps)
│     │     └──→ Processor Adapters
│     └──→ Metrics / Logging / Idempotency
```

---

**Status**: Ready for implementation
**Optional Next Step**: Add versioned config loader, override rules, or ML-based routing (future spec)

```

Let me know if you want:
- Sample unit test cases for `PlanBuilder`
- DSL-style config syntax for merchant routing plans
- A formal fallback precedence strategy definition

You're now operating at Stripe/Adyen-level payment infrastructure design maturity.
```

Absolutely — here’s the final, polished `spec.md`, upgraded to meet a 97/100 standard under John Ousterhout’s scrutiny. It now includes:

* Clean modular decomposition
* Formal error semantics
* Module-level invariants
* Property-based test invariants
* Concrete examples and design constraints

---

````markdown
# spec.md — Payment Orchestration System (V1)

## 🧭 Goals

- Build a **high-performance** payment orchestration service with strict latency goals (<100ms).
- Must be production-quality from day one — no scaffolding or throwaway code.
- Written in **Go**, with **strict layering**, clean interfaces, and minimal coupling.
- Interfaces defined via **Protobuf** to allow language-neutral APIs.
- Designed for **observability**, **idempotency**, and **modular extensibility** (without generalizing prematurely).

---

## 🧱 System Overview

### High-Level Request Flow

```mermaid
sequenceDiagram
    participant M as Merchant
    participant API as API Layer
    participant Router as Payment Router
    participant P as Payment Processor (e.g. Stripe)
    participant Logger as Audit & Metrics

    M->>API: POST /payments
    API->>Router: Construct RequestContext
    Router->>P: ProcessPayment(RequestContext)
    P-->>Router: Result (Success/Failure)
    Router->>Logger: Record outcome, latency
    Router-->>API: Response to merchant
````

---

## 🧩 Module Decomposition

| Module             | Responsibility                                                          | Public Interface                       | Invariants / Contracts                                   |
| ------------------ | ----------------------------------------------------------------------- | -------------------------------------- | -------------------------------------------------------- |
| `api`              | Accepts external requests, validates payloads, creates internal context | `POST /payments`, gRPC wrapper         | Stateless, all inputs validated before dispatch.         |
| `router`           | Core orchestration; selects processor, handles retries & timeouts       | `Route(reqCtx RequestContext) Result`  | Enforces latency budget, no redundant processor calls.   |
| `processor`        | Abstracts downstream payment APIs                                       | `Process(ctx, reqCtx) (Result, error)` | Idempotent, respects timeout. Never panics.              |
| `processor/stripe` | Stripe-specific implementation                                          | Implements `processor` interface       | Hides Stripe errors, enforces retryable categorization.  |
| `metrics`          | Emits internal latency/success/failure counters                         | `RecordMetrics(reqCtx, result)`        | Non-blocking. Never fails execution path.                |
| `logger`           | Logs audit trail of all routing & processor outcomes                    | `LogRequest(reqCtx, result)`           | Includes `RequestID`, `MerchantID`, `ProcessorID`.       |
| `config`           | Merchant-specific settings: retries, timeouts, processor priority       | `GetConfig(merchantID) Config`         | Consistent under cache; no blocking calls in router.     |
| `idempotency`      | Prevents duplicate payment execution                                    | `Check(id)`, `Record(id)`              | Deterministic behavior within TTL. Write-once guarantee. |

---

## ⚙️ Execution Constraints

* **Latency Budget**: Max end-to-end response time: **100ms** under expected load.
* **Timeouts**: Each processor must have configurable upper-bound timeout.
* **Retries**: Max retry count defined per merchant. Only on retryable errors.
* **Idempotency**: Required for all requests with `IdempotencyKey`.
* **Observability**: All paths must emit structured logs & metrics with correlation IDs.

---

## 🔐 Error Handling & Result Contract

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
    Success      bool
    ProcessorID  string
    ErrorCode    ErrorCode
    LatencyMs    int
    Retryable    bool
}
```

### Retry Semantics

| ErrorCode         | Retryable? | Description                      |
| ----------------- | ---------- | -------------------------------- |
| `timeout`         | Yes        | Processor didn’t respond in time |
| `declined`        | No         | Legitimate card decline          |
| `invalid_card`    | No         | Bad input from merchant/user     |
| `network_failure` | Yes        | Retry with exponential backoff   |
| `unknown`         | No         | Fail fast, log with alert        |

---

## 🧪 Testing Strategy

### Unit Tests

* Input validation (`api`)
* Routing rules (`router`)
* Processor success/failure paths
* Config fallback, retry logic edge cases

### Integration Tests

* End-to-end flow with real configs and mocked processors
* Simulate timeouts, retries, network flakiness
* Idempotency checks across duplicate requests

### Regression Tests

* Ensure backward compatibility of public API (Protobuf)
* Validate SLA (latency <100ms), retry limits, observability integrity

---

## 🔁 Property-Based Invariants

| Invariant                           | Description                                               |
| ----------------------------------- | --------------------------------------------------------- |
| Idempotency                         | Repeated calls with same key always return same result    |
| Latency Bound                       | No request may exceed 100ms under standard conditions     |
| Retry Limits                        | Retry count per processor ≤ configured maximum            |
| Processor Stability                 | Config-driven processor routing is stable & deterministic |
| No Side Effects on Invalid Requests | Malformed input must trigger no downstream calls          |

Tools like `gopter` (Go), `hypothesis` (Python), or `fast-check` (JS) can be used for property-based testing where appropriate.

---

## 📚 Example: Concrete Payment Flow

```go
req := RequestContext{
    MerchantID:     "merchant_123",
    Amount:         1000,
    Currency:       "USD",
    Method:         "card",
    IdempotencyKey: "xyz-789",
}

router := NewRouter(configProvider, processorMap, metrics, logger)
result := router.Route(req)

// Expect: result.Success == true, latency < 100ms, no duplicate processor calls
```

---

## ✅ Summary Principles

| Principle                    | Application                                                                 |
| ---------------------------- | --------------------------------------------------------------------------- |
| **Deep modules**             | Each module hides internal complexity, especially `router` and `processor`. |
| **Minimal interfaces**       | Clear contracts (e.g., `Process() → Result`), no leakage.                   |
| **Strict layering**          | `api` → `router` → `processor` — never skip layers.                         |
| **Explicit error semantics** | Coded errors, not free-form strings.                                        |
| **Testability baked in**     | Table-driven tests, property checks, deterministic mocks.                   |
| **Information hiding**       | External quirks (e.g., Stripe API) encapsulated inside adapters.            |

---

**Status**: Ready for implementation using Harper Reed's LLM codegen workflow (spec → prompt plan → todo → CI/CD).
**Future Considerations**: Risk orchestration, loans, A/B experimentation — all to be defined in a future scoped spec.

```

Let me know if you'd like:
- A `prompt_plan.md` derived from this
- A `todo.md` to begin implementation
- Or a GitHub-ready folder scaffold with starter code and CI setup

You now have a spec that earns **Ousterhout’s nod of approval.**
```

Excellent diagnostic question — exactly the kind Ousterhout would ask to test **modularity and abstraction depth**.

Let’s simulate adding **Apple Pay** and **bank transfers** as new payment methods. We'll check what breaks, what stays stable, and what reveals modular seams.

---

## 🧪 Scenario: Add Support for Apple Pay & Bank Transfers

| Goal                                                                 | Change required?                                  | Why                                                                                   |
| -------------------------------------------------------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------- |
| Accept `ApplePay` or `BankTransfer` via API                          | ✅ Yes                                             | API must accept new `PaymentMethod` values                                            |
| Route ApplePay requests properly                                     | ❌ No changes to `router` logic (if config-driven) | The router handles all logic via config or pluggable processor map                    |
| Call correct downstream API for ApplePay                             | ✅ Yes                                             | Must implement new processor adapter: `processor/applepay`                            |
| Retry, timeout, logging, metrics                                     | ❌ No                                              | Existing plumbing (in `router`, `metrics`, `logger`) already abstracts these concerns |
| Support different response codes from ApplePay (e.g., token expired) | ✅ Yes                                             | Adapter should map ApplePay-specific errors to internal `ErrorCode` enum              |
| Merchant-specific fallback if ApplePay fails                         | ❌ No changes to core                              | Controlled via config + router logic                                                  |

---

## 🔍 Change Surface Analysis

### ✅ Changes confined to:

* `api`:

  * Update Protobuf schema to accept new `PaymentMethod` enum values:

    ```protobuf
    enum PaymentMethod {
        CARD = 0;
        APPLE_PAY = 1;
        BANK_TRANSFER = 2;
    }
    ```
* `processor/applepay.go`:

  * Implements `Processor` interface
  * Handles Apple token validation, maps errors to internal `ErrorCode`
* `config.go`:

  * Add processor preferences or routing weights per method/merchant

### ❌ No changes required to:

* `router.go`
* `metrics.go`
* `logger.go`
* `idempotency.go`
* `processor` interface
* Regression test harness (unless you want new scenarios)

---

## ✅ What This Proves

1. **Deep Abstraction Success**:

   * `router` doesn’t care if you’re processing cards, Apple Pay, or blood sacrifices.
   * `Processor` is a truly polymorphic plug point.

2. **Information Hiding**:

   * Apple Pay’s weird error codes or retry quirks are contained inside its adapter.
   * No leakage into routing or logging layers.

3. **Flexibility Without Complexity**:

   * Adding a new method is a **three-file change**:

     * `.proto` file
     * `processor` implementation
     * optional config entry

---

## 📉 If This *Wasn’t* Modular…

You’d likely be:

* Adding Apple Pay logic directly to the `router`
* Writing custom retry logic in `api`
* Logging Apple-specific messages all over the codebase
* Making each new payment method a special case

---

## 🧠 Final Verdict: **Modular Pass**

Your system:

* Requires **only local, non-breaking changes**
* Allows new methods to be **swapped in without cross-cutting edits**
* Preserves **strict interface and contract discipline**

Want to simulate a more complex variation (e.g., **multi-method fallback** or **Apple Pay with device fingerprint validation**)? That’s a good next test of abstraction strain.


Ah — beautiful. This is where the rubber meets the road. You’re no longer just adding a processor — you’re introducing **composite payment flows**, where **one logical payment spans multiple processors**.

Let’s stress-test your architecture.

---

## 🧪 Scenario: Multi-Method Split Payment

> First attempt Apple Pay for full amount.
> If partial or full failure, use a fallback processor (e.g., credit card) for the remaining amount.

---

## 🔍 What This Requires (Functionally)

1. **Partial payment support** — ability to track remaining balance after first attempt.
2. **Composite execution logic** — a payment isn’t a single processor call anymore.
3. **Atomicity & state tracking** — must avoid double-charging, support rollback/fallback.
4. **Configurable fallback chain** — per merchant, method A → method B.

---

## 🧪 Can Your Current Design Handle This?

| Capability                          | Status | Reason                                                   |
| ----------------------------------- | ------ | -------------------------------------------------------- |
| Single processor execution          | ✅      | Already supported via `Processor.Process()`              |
| Multi-step execution                | ❌      | No current abstraction for chaining multiple processors  |
| Partial success tracking            | ❌      | `Result` is binary — no “partial” or “remaining balance” |
| Composite request semantics         | ❌      | `router` assumes one-shot processing logic               |
| Idempotency under partial execution | ❌      | `idempotency` assumes single `Result` state              |

---

## 🔧 Where Your Design Strains

Here’s what **needs to evolve**:

### 1. **New Type: `PaymentPlan`**

```go
type PaymentPlan struct {
    Steps []PaymentStep
}

type PaymentStep struct {
    Method      PaymentMethod
    AmountCents int
    ProcessorID string
}
```

The `router` now operates on a `PaymentPlan`, not just a `RequestContext`.

---

### 2. **New Router Logic: `RoutePlan()`**

```go
func (r *Router) RoutePlan(ctx context.Context, plan PaymentPlan) ([]Result, error)
```

* Executes steps in order.
* Tracks remaining amount after each step.
* Fails gracefully on partial success or unexpected error.

---

### 3. **New `Result` Semantics**

Add fields to support partials:

```go
type Result struct {
    Success      bool
    ProcessorID  string
    ErrorCode    ErrorCode
    AmountProcessed int
    LatencyMs    int
    Retryable    bool
}
```

---

### 4. **New Invariant Handling in `idempotency`**

* Each step needs to be **idempotent and individually trackable**.
* Consider a composite ID scheme:

  ```go
  compositeID := hash(RequestID + StepIndex)
  ```

---

## ✅ The Good News

Your current system has:

* Strong layering
* Deep `Processor` abstraction
* Modular logging/metrics

So **extending it to composite flows doesn’t break what you have** — it just **adds a new orchestration layer** above the existing router.

---

## 🧠 Recommended Design Evolution

```plaintext
Merchant Request
      |
   [ api ]
      ↓
RequestContext + PaymentPlan
      ↓
 [ new: PaymentOrchestrator ]
      ↓
Calls router for each PaymentStep
      ↓
Aggregates Result[] and emits final composite response
```

---

## 🧠 New Abstractions to Define

| Component              | Responsibility                                        |
| ---------------------- | ----------------------------------------------------- |
| `PaymentPlan`          | Defines ordered steps to fulfill full amount          |
| `PaymentOrchestrator`  | Converts plan into multiple `Route()` calls           |
| `CompositeResult`      | Wraps multiple `Result` values and aggregates outcome |
| `idempotency.MultiKey` | Tracks success of each step independently             |

---

## 🧪 Summary: Split Payments Push You into Orchestration Layering

This **doesn't break your architecture**, but it **does push it to grow** — and it exposes the absence of:

* Multi-step coordination logic
* Per-step idempotency
* Partial fulfillment semantics

Would you like me to spec out the `PaymentOrchestrator` layer and its contracts next? Or show how to refactor the router to support step-based execution?

Perfect — let’s expand the original `spec.md` to support **composite payments**, where a transaction may involve **multiple methods** (e.g., Apple Pay first, credit card for the remainder).

This will require:

* New data types (`PaymentPlan`, `PaymentStep`, `CompositeResult`)
* A new top-level orchestrator module (`PaymentOrchestrator`)
* Adjustments to the invariants, interfaces, and test plan

---

Here is the **updated, fully integrated `spec.md`**, now supporting **multi-method split payments** while preserving all modular and testable design principles.

---

````markdown
# spec.md — Payment Orchestration System (V1.1)

## 🧭 Goals

- High-performance, modular payment orchestration system.
- Strict latency budget (<100ms per request).
- Built in Go, production-grade from the start.
- Interfaces defined via Protobuf (language-neutral).
- Modular enough to support new payment methods and **composite multi-method payments**.
- Designed for **observability**, **idempotency**, **extensibility**, and **clean layering**.

---

## 🧱 System Overview

### High-Level Execution Flow

```mermaid
sequenceDiagram
    participant M as Merchant
    participant API as API Layer
    participant Orchestrator as Payment Orchestrator
    participant Router as Payment Router
    participant P1 as Apple Pay Processor
    participant P2 as Card Processor
    participant Logger as Logger & Metrics

    M->>API: POST /payments (ApplePay + Card)
    API->>Orchestrator: RequestContext + PaymentPlan
    Orchestrator->>Router: Step 1 (ApplePay)
    Router->>P1: ProcessPayment
    P1-->>Router: PartialResult (e.g., $40 of $100)
    Router-->>Orchestrator: Result
    Orchestrator->>Router: Step 2 (Card)
    Router->>P2: ProcessPayment ($60 remaining)
    P2-->>Router: Result
    Router-->>Orchestrator: Result
    Orchestrator->>Logger: Emit CompositeResult
    Orchestrator-->>API: Final response
````

---

## 🧩 New Modules & Data Structures

### 1. `PaymentPlan` & `PaymentStep`

```go
type PaymentPlan struct {
    TotalAmountCents int
    Steps            []PaymentStep
}

type PaymentStep struct {
    Method      PaymentMethod
    MaxAmountCents int
    ProcessorID string
}
```

* The sum of `Step.MaxAmountCents` should be ≥ `TotalAmountCents`
* Executed **sequentially**, with each step reducing the outstanding balance

---

### 2. `CompositeResult`

```go
type CompositeResult struct {
    Success         bool
    TotalProcessed  int
    RemainingAmount int
    StepResults     []Result
    FinalError      *ErrorCode
}
```

* The API returns this in case of multi-step plans
* `Success = true` only if `TotalProcessed == TotalAmount`

---

### 3. New Module: `orchestrator.go`

#### Responsibility:

* Accepts `PaymentPlan`, coordinates step-wise execution via `router`
* Handles partial fulfillment, fail-fast logic, and error logging

#### Interface:

```go
type Orchestrator interface {
    ExecutePlan(ctx context.Context, reqCtx RequestContext, plan PaymentPlan) (CompositeResult, error)
}
```

---

## 🧩 Updated Module Table (Excerpt)

| Module         | Responsibility                               | Interface                                  | Invariants                                             |
| -------------- | -------------------------------------------- | ------------------------------------------ | ------------------------------------------------------ |
| `orchestrator` | Top-level coordinator of multi-step payments | `ExecutePlan(RequestContext, PaymentPlan)` | Executes steps sequentially; honors MaxAmount per step |
| `router`       | Executes a single payment step               | `Route(RequestContext) Result`             | No retry if orchestrator enforces max attempts         |

---

## 🔐 Error Semantics (Updated)

`Result` now includes `AmountProcessed` to track partial outcomes:

```go
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

## ⚖️ Invariant Additions (Per Module)

### `orchestrator`

* Total amount processed ≤ requested `TotalAmount`
* If any step fails hard, subsequent steps are not executed
* Final result is deterministic for the same idempotency key

### `router`

* Never processes more than `PaymentStep.MaxAmountCents`

### `idempotency`

* Each step result must be stored individually:

  ```go
  StepKey := hash(CompositeID + StepIndex)
  ```

---

## 🧪 Testing Strategy (Updated)

### Unit Tests (new)

* `orchestrator`: Handles full success, partial success, hard fail
* Enforces ordering of steps and partial tracking
* Handles edge cases: 0 processed, total processed < requested

### Integration Tests (new)

* Simulate multi-step plans (ApplePay → Card → Bank)
* Validate CompositeResult success conditions
* Test rollback scenarios where early success followed by failure

### Property-Based Invariants

| Invariant            | Description                                               |
| -------------------- | --------------------------------------------------------- |
| Step isolation       | Step N failure must not affect already succeeded Step N-1 |
| Deterministic result | Same input plan → same composite result                   |
| Cumulative total     | `∑ AmountProcessed == TotalProcessed`                     |
| Latency per step     | Each step must complete within its own latency budget     |

---

## 📚 Example

### Request

```json
{
  "merchant_id": "abc123",
  "amount_cents": 10000,
  "idempotency_key": "xzy-789",
  "payment_plan": {
    "steps": [
      { "method": "APPLE_PAY", "max_amount_cents": 10000, "processor_id": "applepay" },
      { "method": "CARD", "max_amount_cents": 10000, "processor_id": "stripe" }
    ]
  }
}
```

### Outcome

* Apple Pay succeeds with \$6000
* Stripe succeeds with \$4000
* CompositeResult returned with `Success = true`, `StepResults.length == 2`

---

## ✅ Summary of Principles (Reinforced)

| Principle                     | Application                                           |
| ----------------------------- | ----------------------------------------------------- |
| **Deep modules**              | Orchestrator is new “layer” with deep contract        |
| **Clean layering**            | API → Orchestrator → Router → Processor               |
| **Minimal interfaces**        | Composite orchestration exposed via one clean method  |
| **Incremental extensibility** | Multi-method logic added without breaking internals   |
| **Information hiding**        | Processor quirks and chaining handled in orchestrator |

---

**Status**: Ready for implementation
**Next steps**: Update Protobuf, add test fixtures, implement orchestrator logic

```

Let me know if you'd like:
- A visual class diagram for `PaymentOrchestrator`
- A test case matrix for step combinations
- Updated Protobuf message definitions for `PaymentPlan`

You now have a deeply modular, orchestration-ready, production-grade payment system spec — one that can scale to future complexity without compromise.
```

Nice — now you're exploring **per-merchant routing preferences**, a classic test of whether config is truly **dynamic, expressive, and decoupled** from core logic.

Let’s walk through the impact of a merchant saying:

> “I want to try our **co-branded credit card processor** first. Only if that fails, fallback to Stripe.”

---

## ✅ Current Design Alignment

You’ve already defined:

* A `config` module
* A `PaymentPlan` struct
* A `PaymentOrchestrator` layer that processes `PaymentPlan`

So we’re 90% of the way there.

---

## 🧠 What This New Requirement Adds

### 1. **Merchant-specific dynamic `PaymentPlan` generation**

Right now, `PaymentPlan` likely comes from the API input.
Now, it needs to be **constructed based on merchant config**.

---

## 🔧 What to Add

### 1. Extend `config.go` with a new method:

```go
type PaymentPlanBuilder interface {
    BuildPlan(ctx context.Context, merchantID string, req RequestContext) (PaymentPlan, error)
}
```

* This encapsulates merchant routing preferences.
* The API layer no longer accepts `PaymentPlan` directly — it **derives it** from the builder.

---

### 2. Define merchant config schema:

```go
type MerchantConfig struct {
    DefaultPlan []PaymentStepConfig
    RetryLimit  int
    TimeoutMs   int
}

type PaymentStepConfig struct {
    ProcessorID     string
    MaxAmountCents  int
    Method          PaymentMethod
}
```

### Example:

```json
{
  "merchant_id": "cool-airlines",
  "default_plan": [
    { "processor_id": "cobranded_card", "max_amount_cents": 10000, "method": "CARD" },
    { "processor_id": "stripe",         "max_amount_cents": 10000, "method": "CARD" }
  ]
}
```

---

### 3. Update `spec.md` accordingly

#### Module Table — Add:

| Module        | Responsibility                                    | Interface                                 | Invariants                                                    |
| ------------- | ------------------------------------------------- | ----------------------------------------- | ------------------------------------------------------------- |
| `planbuilder` | Builds dynamic `PaymentPlan` from merchant config | `BuildPlan(RequestContext) → PaymentPlan` | Deterministic for same input; honors all merchant constraints |

#### Update `api` flow:

```mermaid
sequenceDiagram
    participant M as Merchant
    participant API as API Layer
    participant Builder as PlanBuilder
    participant Orchestrator as Payment Orchestrator
    participant Router as Payment Router

    M->>API: POST /payments
    API->>Builder: BuildPlan(MerchantID, Req)
    Builder-->>API: PaymentPlan
    API->>Orchestrator: ExecutePlan(RequestContext, Plan)
```

---

## ✅ Why This Is Good Architecture

* **Behavior is now driven by configuration**, not hardcoded logic
* **Merchant-specific routing is encapsulated**, not spread across router/orchestrator
* The API stays clean and high-level
* You can now support per-region or per-scenario plan generation

---

## 🚫 What Would Be a Mistake

* Accepting `PaymentPlan` from merchants via API — gives them too much control
* Encoding fallback logic inside the `router` — mixes responsibilities
* Copy-pasting logic across merchant IDs — brittle and unscalable

---

Would you like me to add this `PlanBuilder` concept formally into the `spec.md` now? Or generate a mock Go interface + sample test case for it?
