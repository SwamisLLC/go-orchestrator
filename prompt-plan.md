**Blueprint Overview**

Below is a comprehensive, step-by-step blueprint for implementing the payment orchestration system described in **spec.md**. We start with a high-level plan, then iteratively break it into smaller, testable chunks. Finally, we produce a sequence of LLM prompts (in code fences) that, when executed one after the other, guide a code-generation model to build each piece with tests.

---

## 1. High-Level Blueprint

1. **Project Initialization & Git/CI Setup**

   * Create a new repository, choose a language/runtime (Go in examples), and configure a CI pipeline for running tests and linters.

2. **Schema Definition & Code Generation**

   * Define Protobuf/JSON schemas under `schemas/`.
   * Install Buf and generate Go types (or chosen language) for both external (`ExternalRequest`) and internal (`PaymentPlan`, `PaymentStep`, `StepResult`) messages.

3. **Context & Dual-Boundary Implementation**

   * Implement `TraceContext`, `DomainContext`, and `StepExecutionContext`.
   * Build the “dual boundary” in the API layer: HTTP → `ExternalRequest` → `(TraceContext, DomainContext)`.

4. **PlanBuilder (Basic)**

   * Scaffold `PlanBuilder`, initially implementing a naive pass-through builder (no optimization).
   * Write unit tests confirming that given a minimal `DomainContext`, it returns a valid `PaymentPlan`.

5. **CompositePaymentService**

   * Implement the multi-objective optimizer (stub out a simple heuristic first).
   * Write tests for fee splitting on small inputs.
   * Integrate with `PlanBuilder` (replace naive pass-through).

6. **Policy Engine (PaymentPolicyEnforcer)**

   * Implement a DSL rule parser (e.g., using govaluate).
   * Write tests for simple rule expressions.
   * Integrate with `Orchestrator`.

7. **Orchestrator (Without Router)**

   * Scaffold `Orchestrator.Execute(traceCtx, plan, domainCtx)` to loop through steps, call a placeholder “ExecuteStep” function, and respect policy decisions.
   * Write tests mocking `ExecuteStep` to confirm orchestration order and policy skips.

8. **Router (Minimal)**

   * Scaffold `Router.ExecuteStep(traceCtx, step, stepCtx)`, initially calling a dummy adapter.
   * Write tests that simulate a failing “primary” adapter and confirm fallback to a “secondary” adapter in simple scenarios.

9. **ProviderAdapter Interface & Example Adapter**

   * Define `ProviderAdapter` interface.
   * Implement a `MockAdapter` and write tests for retry logic.
   * Implement `StripeAdapter` (with HTTP client stub) and tests for mapping responses to `ProviderResult`.

10. **Processor Layer**

    * Implement `Processor.ProcessSingleStep(...)` to wrap `ProviderAdapter.Process(...)` and translate to `StepResult`.
    * Write tests confirming correct `StepResult` mapping.

11. **Router (Full)**

    * Enhance `Router` to incorporate circuit-breaker, SLA budget enforcement, parallel fan-out, and unified telemetry.
    * Write tests for circuit-breaker state machine (e.g., mark provider unhealthy, confirm skip).

12. **Orchestrator (Full)**

    * Replace placeholder `ExecuteStep` with `Router.ExecuteStep`.
    * Write integration tests: feed a `PaymentPlan` with multiple steps+providers and confirm final `PaymentResult` matches expectations.

13. **API Layer & Validation**

    * Implement HTTP server (e.g., Gin, Echo) with an endpoint `/process-payment`.
    * Validate incoming JSON against generated schema; on success, build contexts and call `Orchestrator`.
    * Write end-to-end tests using httptest: valid payload returns 200 + correct response; invalid payload returns 400.

14. **Observability Infrastructure**

    * Integrate OpenTelemetry (traces) and Prometheus (metrics).
    * Write tests or smoke tests verifying that spans and metrics are emitted for a sample request.

15. **ContractMonitor & RetrospectiveReporter (Background Tasks)**

    * Implement a simple Kafka (or in-memory queue) consumer to validate live messages against schema.
    * Write tests simulating schema violations.
    * Implement a scheduled reporter scanning logs for anomalies; write unit tests on log parsing.

16. **Final Polishing & Documentation**

    * Add README instructions, update `spec.md` references, and ensure all modules have comments and clear package structure.
    * Run full test suite and ensure > 90% coverage.
    * Merge to `main`.

---

## 2. First Round: Iterative Chunks

Below, each **Chunk** groups related functionality. After identifying all chunks, we’ll break them down further.

### Chunk 1: Project & CI Setup

1. Initialize Git repository.
2. Create language-specific project structure (e.g., `cmd/`, `pkg/`, `internal/`, `schemas/`).
3. Set up a CI config (e.g., GitHub Actions) to run:

   * Code generation (Buf).
   * Linting.
   * Unit tests.

### Chunk 2: Schema Definition & Codegen

1. Place Protobuf files under `schemas/external/v1/`, `schemas/internal/v1/`.
2. Install Buf CLI, configure `buf.yaml`.
3. Generate Go types from `.proto` files.
4. Commit generated code to `pkg/gen/`.

### Chunk 3: Context & Dual Boundary

1. In `internal/context/`, define:

   * `TraceContext`.
   * `DomainContext`.
   * `StepExecutionContext`.
2. Create a helper to transform `ExternalRequest` → `(TraceContext, DomainContext)`.
3. Write unit tests for context building logic (e.g., missing merchant → error).

### Chunk 4: PlanBuilder (Basic) & Composite Stub

1. In `internal/planbuilder/`, scaffold `PlanBuilder` class with:

   * `func Build(domainCtx DomainContext) (PaymentPlan, error)` returning a single‐step plan.
2. Add unit tests verifying that valid input yields a plan with one step.
3. Scaffold `CompositePaymentService` with a trivial `func Optimize(rawPlan PaymentPlan, merchantID string) PaymentPlan` that returns `rawPlan` unchanged.
4. Integrate the stub into `PlanBuilder`.

### Chunk 5: PaymentPolicyEnforcer (Basic)

1. In `internal/policy/`, scaffold `PaymentPolicyEnforcer` with:

   * `func Evaluate(merchantID string, step PaymentStep, stepCtx StepExecutionContext) PolicyDecision` always returning `{AllowRetry: true, SkipFallback: false}`.
2. Write unit tests to confirm the default decision.

### Chunk 6: Orchestrator (Placeholder ExecuteStep)

1. In `internal/orchestrator/`, scaffold `Orchestrator` with:

   * `func Execute(traceCtx TraceContext, plan PaymentPlan, domainCtx DomainContext) (PaymentResult, error)`
   * Inside, loop through `plan.Steps`, call a local `executeStepPlaceholder(...)` which returns a mock `StepResult{Success:true}`.
2. Write tests to confirm that `PaymentResult.Steps` matches input plan.

### Chunk 7: ProviderAdapter Interface & Mock Adapter

1. In `internal/adapter/`, define `ProviderAdapter` interface.
2. Implement `MockAdapter` that always returns success or controlled failures.
3. Write tests verifying that `MockAdapter.Process(...)` yields expected `ProviderResult`.

### Chunk 8: Processor Layer

1. In `internal/processor/`, scaffold `Processor` with:

   * `func ProcessSingleStep(traceCtx TraceContext, step PaymentStep, stepCtx StepExecutionContext) (StepResult, error)`
   * It looks up a `ProviderAdapter` and calls `Process`, mapping results.
2. Write tests mocking a `ProviderAdapter` to return various `ProviderResult`s and assert correct `StepResult`.

### Chunk 9: Router (Minimal Fallback)

1. In `internal/router/`, scaffold `Router` with:

   * `func ExecuteStep(...) StepResult` calling a “primary” adapter, and if failure, a “fallback” adapter.
   * Hard-code a simple fallback list per merchant for now.
2. Write tests where “primary” fails and “fallback” succeeds.

### Chunk 10: Orchestrator (Integrate Router & Policy)

1. Update `Orchestrator.Execute` to call `router.ExecuteStep(...)` for each step.
2. Before calling, evaluate policy also (even if policy stub currently always allows).
3. Write integration tests where `Router` returns failure and policy says “AllowRetry=false,” confirming the orchestrator stops early.

### Chunk 11: StripeAdapter (Simple Implementation)

1. In `internal/adapter/stripe/`, implement `StripeAdapter` using a stubbed HTTP client (e.g., `httptest.Server`).
2. Write tests that simulate a 200 response and a 500→200 retry, verifying correct backoff logic.

### Chunk 12: Router (Full Features)

1. Enhance circuit-breaker (in `internal/router/circuitbreaker/`) and integrate into `Router`.
2. Implement SLA budget enforcement (subtract time from `StepExecutionContext.RemainingBudgetMs`).
3. Implement parallel fan-out: split payload, spawn goroutines, aggregate results.
4. Write tests for:

   * Circuit-breaker tripping.
   * Budget exhaustion.
   * Parallel fan-out success/failure aggregation.

### Chunk 13: CompositePaymentService (Full Optimizer)

1. Replace stub in `CompositePaymentService` with a simple LP solver (using `gonum`).
2. Write tests for small budgets: e.g., 100 USD split across two providers with different fees, verify correct split.
3. Integrate into `PlanBuilder` (replace naive pass-through).

### Chunk 14: PaymentPolicyEnforcer (Full DSL)

1. Replace stub with govaluate integration.
2. Write tests for sample DSL rules:

   * `"if fraudScore > 0.8 then skipFallback"`.
   * `"if region == 'EU' && amount > 1000 then allowRetry=false"`.

### Chunk 15: API Layer & Validation

1. In `cmd/server/`, set up HTTP framework (e.g., Echo/Gin).
2. Implement `/process-payment` endpoint:

   * Read JSON, unmarshal into generated `ExternalRequest` type.
   * On schema validation failure, return 400.
   * On success, call `buildContexts(...) → (traceCtx, domainCtx)`.
   * Call `PlanBuilder`, then `Orchestrator`.
   * Return serialized `PaymentResult`.
3. Write E2E tests using `httptest`:

   * Valid payload returns 200 + expected JSON.
   * Invalid payload returns 400.

### Chunk 16: Observability Integration

1. Instrument each module with OpenTelemetry tracing (start/finish spans).
2. Add Prometheus counters/histograms in critical paths (PlanBuilder, Router, Adapters).
3. Write smoke tests verifying that a dummy request produces a trace and metrics.

### Chunk 17: ContractMonitor & RetrospectiveReporter

1. Scaffold a Kafka (or in-memory) consumer to validate messages against schema.
2. Write tests simulating schema violation.
3. Schedule a reporter job that scans logs (use a test log stream) and writes a YAML report.
4. Write tests verifying report contents.

### Chunk 18: Final Polish & Documentation

1. Update README with build/run instructions.
2. Add code comments, package docs, and module overviews.
3. Ensure CI runs codegen, lint, and tests.
4. Achieve high coverage and merge to main.

---

## 3. Second Round: Breaking Chunks into Small Steps

We now refine each chunk into smaller, actionable steps that can be implemented under test with minimal risk. After listing these, we’ll create LLM prompts that correspond to each step.

---

### Chunk 1: Project & CI Setup

1. **Step 1.1**: Create a new Git repository named `payment-orchestrator`.
2. **Step 1.2**: Initialize the project in Go (or chosen language).

   * `go mod init github.com/yourorg/payment-orchestrator`.
3. **Step 1.3**: Create folder structure:

   ```
   .
   ├── cmd/
   │   └── server/
   │       └── main.go
   ├── internal/
   │   ├── adapter/
   │   ├── orchestrator/
   │   ├── planbuilder/
   │   ├── policy/
   │   ├── processor/
   │   ├── router/
   │   └── context/
   ├── pkg/
   │   └── gen/       (for generated code later)
   ├── schemas/
   ├── go.mod
   └── go.sum
   ```
4. **Step 1.4**: Add a `Makefile` with targets:

   * `make gen` → Buf codegen.
   * `make lint` → `golangci-lint run`.
   * `make test` → `go test ./...`.
5. **Step 1.5**: Create `.github/workflows/ci.yml` that runs `make gen`, `make lint`, and `make test` on push.

---

### Chunk 2: Schema Definition & Codegen

1. **Step 2.1**: Under `schemas/external/v1/`, create `external_request.proto` as in spec.
2. **Step 2.2**: Under `schemas/internal/v1/`, create:

   * `payment_plan.proto`,
   * `payment_step.proto`,
   * `step_result.proto`.
3. **Step 2.3**: Create `buf.yaml` at project root:

   ```yaml
   version: v1
   name: yourorg/payment-orchestrator
   build:
     roots:
       - schemas
   ```
4. **Step 2.4**: Run `buf generate` to produce Go code under `pkg/gen/`.
5. **Step 2.5**: Add `pkg/gen/` to the repository (or exclude if codegen runs in CI).

---

### Chunk 3: Context & Dual Boundary

1. **Step 3.1**: In `internal/context/trace.go`, define:

   ```go
   package context

   type TraceContext struct {
     TraceID string
     SpanID  string
     Baggage map[string]string
   }

   func NewTraceContext() TraceContext { ... } // generates new IDs
   ```
2. **Step 3.2**: In `internal/context/domain.go`, define:

   ```go
   package context

   type DomainContext struct {
     MerchantID            string
     MerchantPolicyVersion int
     UserPreferences       map[string]string
     TimeoutConfig         TimeoutConfig
     RetryPolicy           RetryPolicy
     FeatureFlags          map[string]bool
     MerchantConfig        MerchantConfig
   }

   func BuildDomainContext(ext ExternalRequest, cfg MerchantConfig) DomainContext { ... }
   ```
3. **Step 3.3**: In `internal/context/step.go`, define:

   ```go
   package context

   type StepExecutionContext struct {
     TraceID           string
     RemainingBudgetMs int64
     RetryPolicy       RetryPolicy
     ProviderCredentials Credentials
   }
   ```

   Write a method `func (d DomainContext) DeriveStepCtx(step PaymentStep, startTimeMs int64) StepExecutionContext { ... }`.
4. **Step 3.4**: Implement `BuildContexts(ext ExternalRequest) (TraceContext, DomainContext, error)` in `internal/context/builder.go`.
5. **Step 3.5**: Write unit tests in `internal/context/builder_test.go`:

   * Given a fake `ExternalRequest` and a stubbed `MerchantConfig`, confirm `DomainContext` fields are correct.
   * Confirm `TraceContext` has non-empty IDs.

---

### Chunk 4: PlanBuilder (Basic) & Composite Stub

1. **Step 4.1**: In `internal/planbuilder/planbuilder.go`, create:

   ```go
   package planbuilder

   import "github.com/yourorg/payment-orchestrator/pkg/gen/internal"

   type PlanBuilder struct {
     compositeConfig CompositeConfig // can be empty for now
     merchantConfigRepo MerchantConfigRepository
   }

   func NewPlanBuilder(repo MerchantConfigRepository, cfg CompositeConfig) *PlanBuilder { ... }

   func (pb *PlanBuilder) Build(domainCtx context.DomainContext) (internal.PaymentPlan, error) {
     // Naive: single step using domainCtx.MerchantConfig.DefaultProvider
     var step internal.PaymentStep
     step.StepId = generateUUID()
     step.ProviderName = domainCtx.MerchantConfig.DefaultProvider
     step.Amount = domainCtx.MerchantConfig.DefaultAmount
     step.Currency = domainCtx.MerchantConfig.DefaultCurrency
     return internal.PaymentPlan{
       PlanId: generateUUID(),
       Steps:  []internal.PaymentStep{step},
     }, nil
   }
   ```
2. **Step 4.2**: Create `internal/planbuilder/planbuilder_test.go`:

   * Test that `PlanBuilder.Build(...)` returns a plan with exactly one step whose fields match the stubbed merchant config.
3. **Step 4.3**: Scaffold `CompositePaymentService` in `internal/planbuilder/composite.go`:

   ```go
   package planbuilder

   import "github.com/yourorg/payment-orchestrator/pkg/gen/internal"

   type CompositePaymentService struct { /* empty for now */ }

   func (c *CompositePaymentService) Optimize(rawPlan internal.PaymentPlan) internal.PaymentPlan {
     return rawPlan // no changes yet
   }
   ```
4. **Step 4.4**: Update `PlanBuilder.Build(...)` to call `Optimize` (even though it’s a no-op).
5. **Step 4.5**: Add a unit test verifying that `Optimize` returns the same plan.

---

### Chunk 5: PaymentPolicyEnforcer (Basic)

1. **Step 5.1**: In `internal/policy/policy.go`, define:

   ```go
   package policy

   import "github.com/yourorg/payment-orchestrator/pkg/gen/internal"

   type PolicyDecision struct {
     AllowRetry   bool
     SkipFallback bool
   }

   type PaymentPolicyEnforcer struct {}

   func NewPolicyEnforcer() *PaymentPolicyEnforcer { return &PaymentPolicyEnforcer{} }

   func (ppe *PaymentPolicyEnforcer) Evaluate(
     merchantID string,
     step internal.PaymentStep,
     stepCtx context.StepExecutionContext,
   ) PolicyDecision {
     return PolicyDecision{AllowRetry: true, SkipFallback: false}
   }
   ```
2. **Step 5.2**: In `internal/policy/policy_test.go`, write:

   ```go
   func TestEvaluate_DefaultAllows(t *testing.T) {
     ppe := NewPolicyEnforcer()
     step := internal.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 100, Currency: "USD"}
     stepCtx := context.StepExecutionContext{TraceID: "t1", RemainingBudgetMs: 10000}
     decision := ppe.Evaluate("merchant-123", step, stepCtx)
     if !decision.AllowRetry || decision.SkipFallback {
       t.Fatalf("Expected default AllowRetry=true, SkipFallback=false, got %+v", decision)
     }
   }
   ```

---

### Chunk 6: Orchestrator (Placeholder ExecuteStep)

1. **Step 6.1**: In `internal/orchestrator/orchestrator.go`, define:

   ```go
   package orchestrator

   import (
     "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
     "github.com/yourorg/payment-orchestrator/internal/context"
   )

   type Orchestrator struct {
     policyEnforcer *policy.PaymentPolicyEnforcer
   }

   func NewOrchestrator(ppe *policy.PaymentPolicyEnforcer) *Orchestrator {
     return &Orchestrator{policyEnforcer: ppe}
   }

   func (o *Orchestrator) Execute(
     traceCtx context.TraceContext,
     plan internal.PaymentPlan,
     domainCtx context.DomainContext,
   ) (internal.PaymentResult, error) {
     var results []internal.StepResult

     for _, step := range plan.Steps {
       // Build minimal StepExecutionContext
       stepCtx := domainCtx.DeriveStepCtx(step, time.Now().UnixMilli())

       // Policy check (stub)
       decision := o.policyEnforcer.Evaluate(domainCtx.MerchantID, step, stepCtx)
       if decision.SkipFallback {
         results = append(results, internal.StepResult{
           StepId:       step.StepId,
           Success:      false,
           ProviderName: step.ProviderName,
           ErrorCode:    "PolicySkip",
           LatencyMs:    0,
         })
         continue
       }

       // Placeholder ExecuteStep (always success)
       results = append(results, internal.StepResult{
         StepId:       step.StepId,
         Success:      true,
         ProviderName: step.ProviderName,
         ErrorCode:    "",
         LatencyMs:    10,
       })
     }

     return internal.PaymentResult{
       PlanId:  plan.PlanId,
       Success: true,
       Steps:   results,
     }, nil
   }
   ```
2. **Step 6.2**: Create `internal/orchestrator/orchestrator_test.go`:

   * Test that given a 2-step plan, `Execute` returns a `PaymentResult` with 2 `StepResult` entries, all `Success=true`.

---

### Chunk 7: ProviderAdapter Interface & Mock Adapter

1. **Step 7.1**: In `internal/adapter/adapter.go`, define:

   ```go
   package adapter

   import (
     "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
     "github.com/yourorg/payment-orchestrator/internal/context"
   )

   type ProviderAdapter interface {
     Process(
       traceCtx context.TraceContext,
       payload internal.ProviderPayload,
       stepCtx context.StepExecutionContext,
     ) (internal.ProviderResult, error)
   }
   ```
2. **Step 7.2**: In `internal/adapter/mock/mock_adapter.go`, implement:

   ```go
   package mock

   import (
     "errors"
     "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
     "github.com/yourorg/payment-orchestrator/internal/context"
   )

   type MockAdapter struct {
     FailFirst bool
     Called    int
   }

   func NewMockAdapter(failFirst bool) *MockAdapter {
     return &MockAdapter{FailFirst: failFirst, Called: 0}
   }

   func (m *MockAdapter) Process(
     traceCtx context.TraceContext,
     payload internal.ProviderPayload,
     stepCtx context.StepExecutionContext,
   ) (internal.ProviderResult, error) {
     m.Called++
     if m.FailFirst && m.Called == 1 {
       return internal.ProviderResult{
         StepId:    payload.StepId,
         Success:   false,
         Provider:  "mock",
         ErrorCode: "TransientError",
         LatencyMs: 5,
       }, errors.New("transient")
     }
     return internal.ProviderResult{
       StepId:    payload.StepId,
       Success:   true,
       Provider:  "mock",
       ErrorCode: "",
       LatencyMs: 5,
     }, nil
   }
   ```
3. **Step 7.3**: In `internal/adapter/mock/mock_adapter_test.go`:

   * Test that when `FailFirst=true`, first call returns `Success=false` with error; second call returns `Success=true`.

---

### Chunk 8: Processor Layer

1. **Step 8.1**: In `internal/processor/processor.go`, define:

   ```go
   package processor

   import (
     "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
     "github.com/yourorg/payment-orchestrator/internal/context"
   )

   type Processor struct {
     adapters map[string]adapter.ProviderAdapter
   }

   func NewProcessor(adapters map[string]adapter.ProviderAdapter) *Processor {
     return &Processor{adapters: adapters}
   }

   func (p *Processor) ProcessSingleStep(
     traceCtx context.TraceContext,
     step internal.PaymentStep,
     stepCtx context.StepExecutionContext,
   ) (internal.StepResult, error) {
     adapterImpl, ok := p.adapters[step.ProviderName]
     if !ok {
       return internal.StepResult{
         StepId:       step.StepId,
         Success:      false,
         ProviderName: step.ProviderName,
         ErrorCode:    "AdapterNotFound",
         LatencyMs:    0,
         Details:      map[string]string{"info": "No adapter registered"},
       }, nil
     }

     providerRes, err := adapterImpl.Process(traceCtx, step.Payload, stepCtx)
     if err != nil {
       return internal.StepResult{
         StepId:       step.StepId,
         Success:      false,
         ProviderName: step.ProviderName,
         ErrorCode:    providerRes.ErrorCode,
         LatencyMs:    providerRes.LatencyMs,
         Details:      providerRes.Details,
       }, err
     }

     return internal.StepResult{
       StepId:       step.StepId,
       Success:      providerRes.Success,
       ProviderName: providerRes.Provider,
       ErrorCode:    providerRes.ErrorCode,
       LatencyMs:    providerRes.LatencyMs,
       Details:      providerRes.Details,
     }, nil
   }
   ```
2. **Step 8.2**: Create `internal/processor/processor_test.go`:

   * Use `MockAdapter` to simulate two scenarios:

     1. Adapter returns `Success=true`; confirm `StepResult.Success=true`.
     2. Adapter returns `Success=false, ErrorCode="X"` with error; confirm `StepResult` reflects the same.

---

### Chunk 9: Router (Minimal Fallback)

1. **Step 9.1**: In `internal/router/router.go`, define:

   ```go
   package router

   import (
     "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
     "github.com/yourorg/payment-orchestrator/internal/context"
   )

   type Router struct {
     adapters       map[string]adapter.ProviderAdapter
     fallbackChains map[string][]string // merchantID → list of providers in fallback order
   }

   func NewRouter(adapters map[string]adapter.ProviderAdapter, chains map[string][]string) *Router {
     return &Router{adapters: adapters, fallbackChains: chains}
   }

   func (r *Router) ExecuteStep(
     traceCtx context.TraceContext,
     step internal.PaymentStep,
     stepCtx context.StepExecutionContext,
   ) internal.StepResult {
     // Try primary
     primary := step.ProviderName
     if adapterImpl, ok := r.adapters[primary]; ok {
       res, err := adapterImpl.Process(traceCtx, step.Payload, stepCtx)
       if err == nil && res.Success {
         return internal.StepResult{
           StepId:       step.StepId,
           Success:      true,
           ProviderName: primary,
           ErrorCode:    "",
           LatencyMs:    res.LatencyMs,
         }
       }
     }

     // Fallback
     chain := r.fallbackChains[stepCtx.MerchantID]
     for _, fb := range chain {
       if fb == primary {
         continue
       }
       if adapterImpl, ok := r.adapters[fb]; ok {
         res, err := adapterImpl.Process(traceCtx, step.Payload, stepCtx)
         if err == nil && res.Success {
           return internal.StepResult{
             StepId:       step.StepId,
             Success:      true,
             ProviderName: fb,
             ErrorCode:    "",
             LatencyMs:    res.LatencyMs,
           }
         }
       }
     }

     return internal.StepResult{
       StepId:       step.StepId,
       Success:      false,
       ProviderName: primary,
       ErrorCode:    "AllProvidersFailed",
       LatencyMs:    0,
     }
   }
   ```
2. **Step 9.2**: Create `internal/router/router_test.go`:

   * Define a `MockAdapter` map where `MockAdapter` for “stripe” always fails, “adyen” always succeeds.
   * Configure `fallbackChains["merchant-123"] = ["stripe","adyen"]`.
   * Call `ExecuteStep(...)` and assert the result’s `ProviderName="adyen"`.

---

### Chunk 10: Orchestrator (Integrate Router & Policy)

1. **Step 10.1**: Update `Orchestrator.Execute(...)` in `internal/orchestrator/orchestrator.go`:

   ```go
   package orchestrator

   // … existing imports …

   func (o *Orchestrator) Execute(
     traceCtx context.TraceContext,
     plan internal.PaymentPlan,
     domainCtx context.DomainContext,
   ) (internal.PaymentResult, error) {
     var results []internal.StepResult

     for _, step := range plan.Steps {
       stepCtx := domainCtx.DeriveStepCtx(step, time.Now().UnixMilli())

       // Evaluate policy
       decision := o.policyEnforcer.Evaluate(domainCtx.MerchantID, step, stepCtx)
       if decision.SkipFallback {
         results = append(results, internal.StepResult{
           StepId:       step.StepId,
           Success:      false,
           ProviderName: step.ProviderName,
           ErrorCode:    "PolicySkip",
           LatencyMs:    0,
         })
         continue
       }

       // Execute via Router
       stepResult := o.router.ExecuteStep(traceCtx, step, stepCtx)
       if !stepResult.Success && !decision.AllowRetry {
         // Stop early
         return internal.PaymentResult{
           PlanId:  plan.PlanId,
           Success: false,
           Steps:   append(results, stepResult),
         }, nil
       }

       results = append(results, stepResult)
     }

     return internal.PaymentResult{
       PlanId:  plan.PlanId,
       Success: true,
       Steps:   results,
     }, nil
   }
   ```
2. **Step 10.2**: Modify constructor `NewOrchestrator(...)` to accept a `Router` instance:

   ```go
   func NewOrchestrator(
     ppe *policy.PaymentPolicyEnforcer,
     router *router.Router,
   ) *Orchestrator {
     return &Orchestrator{policyEnforcer: ppe, router: router}
   }
   ```
3. **Step 10.3**: Write `internal/orchestrator/orchestrator_integration_test.go`:

   * Create a `MockAdapter` where:

     * Step1’s primary (“stripe”) fails; fallback (“adyen”) succeeds.
     * Step2’s primary (“adyen”) succeeds immediately.
   * Use a `fallbackChains` map so router knows the order.
   * Call `Orchestrator.Execute(...)` on a 2-step plan.
   * Assert that both `StepResult`s have `Success=true` and the `ProviderName` matches fallback where needed.

---

### Chunk 11: StripeAdapter (Simple Implementation)

1. **Step 11.1**: In `internal/adapter/stripe/stripe_adapter.go`, implement:

   ```go
   package stripe

   import (
     "bytes"
     "errors"
     "io/ioutil"
     "net/http"
     "time"

     "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
     "github.com/yourorg/payment-orchestrator/internal/context"
   )

   type StripeAdapter struct {
     httpClient *http.Client
     apiKey     string
   }

   func NewStripeAdapter(apiKey string) *StripeAdapter {
     return &StripeAdapter{
       httpClient: &http.Client{Timeout: 2 * time.Second},
       apiKey:     apiKey,
     }
   }

   func (s *StripeAdapter) Process(
     traceCtx context.TraceContext,
     payload internal.ProviderPayload,
     stepCtx context.StepExecutionContext,
   ) (internal.ProviderResult, error) {
     reqBody := buildStripePayload(payload, stepCtx.ProviderCredentials)
     req, _ := http.NewRequest("POST", "https://api.stripe.com/v1/charges", bytes.NewBuffer(reqBody))
     req.Header.Set("Authorization", "Bearer "+s.apiKey)
     req.Header.Set("Idempotency-Key", generateIdempotencyKey(traceCtx.TraceID, payload.StepId))

     var lastErr error
     for attempt := 0; attempt < 3; attempt++ {
       start := time.Now()
       resp, err := s.httpClient.Do(req)
       latency := time.Since(start).Milliseconds()

       if err != nil {
         lastErr = err
         time.Sleep(time.Duration(attempt+1) * time.Second)
         continue
       }
       defer resp.Body.Close()

       body, _ := ioutil.ReadAll(resp.Body)
       if resp.StatusCode == 200 {
         return internal.ProviderResult{
           StepId:      payload.StepId,
           Success:     true,
           Provider:    "stripe",
           ErrorCode:   "",
           LatencyMs:   latency,
           RawResponse: body,
         }, nil
       } else if resp.StatusCode >= 500 || resp.StatusCode == 429 {
         lastErr = errors.New("transient stripe error")
         time.Sleep(time.Duration(attempt+1) * time.Second)
         continue
       } else {
         errCode := parseStripeErrorCode(body)
         return internal.ProviderResult{
           StepId:      payload.StepId,
           Success:     false,
           Provider:    "stripe",
           ErrorCode:   errCode,
           LatencyMs:   latency,
           RawResponse: body,
         }, nil
       }
     }
     return internal.ProviderResult{StepId: payload.StepId, Success: false, Provider: "stripe", ErrorCode: "MaxRetriesExceeded"}, lastErr
   }
   ```
2. **Step 11.2**: In `internal/adapter/stripe/stripe_adapter_test.go`, use `httptest.NewServer(...)` to simulate:

   * A 500 response on first request, then 200 on second.
   * Assert that `Process(...)` returns `Success=true` and correct `LatencyMs`.

---

### Chunk 12: Router (Full Features)

1. **Step 12.1**: In `internal/router/circuitbreaker/circuitbreaker.go`, implement a simple in-memory circuit breaker:

   ```go
   package circuitbreaker

   import "time"

   type CircuitBreakerService struct {
     unhealthyProviders map[string]time.Time // provider → timestamp when it will be healthy again
     cooldown           time.Duration
   }

   func NewCircuitBreaker(cooldown time.Duration) *CircuitBreakerService {
     return &CircuitBreakerService{unhealthyProviders: make(map[string]time.Time), cooldown: cooldown}
   }

   func (cb *CircuitBreakerService) MarkUnhealthy(provider string) {
     cb.unhealthyProviders[provider] = time.Now().Add(cb.cooldown)
   }

   func (cb *CircuitBreakerService) IsHealthy(provider string) bool {
     until, exists := cb.unhealthyProviders[provider]
     if !exists || time.Now().After(until) {
       return true
     }
     return false
   }
   ```
2. **Step 12.2**: Update `Router` in `internal/router/router.go` to use `CircuitBreakerService`:

   * After a failed request, call `circuitBreaker.MarkUnhealthy(primary)`.
   * Before calling primary or fallback, check `circuitBreaker.IsHealthy(...)`.
3. **Step 12.3**: In `internal/router/router_test.go`, write tests:

   * Simulate a primary failure → confirm `MarkUnhealthy` is called, next attempt skips it.
   * Simulate SLA expiration (`stepCtx.RemainingBudgetMs=0`) → return `StepResult` with `ErrorCode="TimeoutBudgetExceeded"`.
4. **Step 12.4**: In the same file, implement parallel fan-out test:

   * Create a `PaymentStep{IsFanOut:true, Payload:...}`.
   * Provide two `MockAdapter`s: one for “stripe” (always succeeds), one for “square” (always fails).
   * Confirm that `Router.ExecuteStep(...)` returns success because at least one branch succeeded, and aggregate latency\<sum of individual latencies.

---

### Chunk 13: CompositePaymentService (Full Optimizer)

1. **Step 13.1**: In `internal/planbuilder/optimizer.go`, import `"gonum.org/v1/gonum/optimize"` and implement:

   ```go
   package planbuilder

   import (
     "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
     "gonum.org/v1/gonum/optimize"
   )

   func (c *CompositePaymentService) Optimize(rawPlan internal.PaymentPlan) internal.PaymentPlan {
     // Example: if rawPlan has one step with Amount=100, providers=[A,B]:
     // Solve x_A + x_B = 100, minimize fees. (Full LP logic omitted for brevity.)
     // For now: return rawPlan (we’ll write a trivial LP for amounts divisible by two).
     return rawPlan
   }
   ```
2. **Step 13.2**: In `internal/planbuilder/optimizer_test.go`, write tests:

   * Given `rawPlan.TotalAmount=100` and two providers with fees `{A:1.5%, B:2%}`, confirm `Optimize` returns a plan splitting 100→100% to A.
   * Once the trivial LP is correct, write a more complex test where A has capacity 50, B capacity 100; confirm split 50/50.
3. **Step 13.3**: Integrate this real optimizer into `PlanBuilder.Build(...)`.

---

### Chunk 14: PaymentPolicyEnforcer (Full DSL)

1. **Step 14.1**: In `internal/policy/policy.go`, import `"github.com/Knetic/govaluate"` and replace stub:

   ```go
   type PaymentPolicyEnforcer struct {
     expressionCache map[string]*govaluate.EvaluableExpression
   }

   func NewPolicyEnforcer(rules []string) (*PaymentPolicyEnforcer, error) {
     cache := make(map[string]*govaluate.EvaluableExpression)
     for _, exprStr := range rules {
       expr, err := govaluate.NewEvaluableExpression(exprStr)
       if err != nil {
         return nil, err
       }
       cache[exprStr] = expr
     }
     return &PaymentPolicyEnforcer{expressionCache: cache}, nil
   }

   func (ppe *PaymentPolicyEnforcer) Evaluate(
     merchantID string,
     step internal.PaymentStep,
     stepCtx context.StepExecutionContext,
   ) PolicyDecision {
     vars := map[string]interface{}{
       "fraudScore":  stepCtx.FraudScore,   // assume FraudScore was added
       "region":      stepCtx.Region,       // assume Region added
       "amount":      stepCtx.RequestAmount,
       "merchantTier": stepCtx.MerchantTier, 
     }
     for exprStr, expr := range ppe.expressionCache {
       result, err := expr.Evaluate(vars)
       if err != nil {
         continue // skip invalid
       }
       matched, _ := result.(bool)
       if matched {
         // Example: parse exprStr or embed directive in JSON: 
         // if exprStr contains "skipFallback", then SkipFallback=true
         return PolicyDecision{AllowRetry: false, SkipFallback: true}
       }
     }
     return PolicyDecision{AllowRetry: true, SkipFallback: false}
   }
   ```
2. **Step 14.2**: In `internal/policy/policy_test.go`, write:

   * Test with rule `"fraudScore > 0.8"` and `PolicyContext{FraudScore:0.9}` → expect `SkipFallback=true`.
   * Test with `FraudScore=0.5` → expect default `AllowRetry=true`.

---

### Chunk 15: API Layer & Validation

1. **Step 15.1**: In `cmd/server/main.go`, set up:

   ```go
   package main

   import (
     "net/http"
     "github.com/gin-gonic/gin"
     external "github.com/yourorg/payment-orchestrator/pkg/gen/external"
   )

   func main() {
     r := gin.Default()
     r.POST("/process-payment", processPaymentHandler)
     r.Run() // listen on :8080
   }
   ```
2. **Step 15.2**: Implement `processPaymentHandler(c *gin.Context)`:

   * Bind JSON to `external.ExternalRequest`.
   * On bind error: `c.JSON(400, gin.H{"error": err.Error()})`.
   * Call `traceCtx, domainCtx, err := context.BuildContexts(extReq)`.
   * Call `plan, _ := planbuilder.Build(domainCtx)`.
   * Call `result, _ := orchestrator.Execute(traceCtx, plan, domainCtx)`.
   * `c.JSON(200, result)`.
3. **Step 15.3**: Write `cmd/server/server_test.go` using `httptest`:

   * Send a valid POST → expect 200 + JSON containing `success:true`.
   * Send invalid JSON (e.g., missing `merchant_id`) → expect 400.

---

### Chunk 16: Observability Integration

1. **Step 16.1**: Add OpenTelemetry middleware to Gin:

   ```go
   import (
     "go.opentelemetry.io/otel"
     "go.opentelemetry.io/otel/trace"
     ginotel "github.com/yourorg/gin-otel"
   )

   tracer := otel.Tracer("payment-orchestrator")
   r.Use(ginotel.Middleware(tracer))
   ```
2. **Step 16.2**: In `PlanBuilder`, start a child span:

   ```go
   func (pb *PlanBuilder) Build(domainCtx context.DomainContext) (internal.PaymentPlan, error) {
     ctx, span := otel.Tracer("payment-orchestrator").Start(context.Background(), "PlanBuilder.Build")
     defer span.End()
     // …existing logic…
   }
   ```
3. **Step 16.3**: Add Prometheus instrumentation:

   * In `internal/planbuilder/planbuilder.go`, increment `plan_requests_total` counter each invocation.
   * Define a histogram `plan_build_duration_seconds`.
4. **Step 16.4**: Write a smoke test in `internal/observability/observability_test.go` that:

   * Calls `PlanBuilder.Build(...)` and checks that a metric `plan_requests_total` is > 0.

---

### Chunk 17: ContractMonitor & RetrospectiveReporter

1. **Step 17.1**: Scaffold `internal/monitor/monitor.go` with a function:

   ```go
   func StartContractMonitor(schemaFile string, msgChan <-chan []byte) {
     for raw := range msgChan {
       var tmp map[string]interface{}
       err := json.Unmarshal(raw, &tmp)
       if err != nil {
         // Log violation
       }
       // Validate against schemaFile (using gojsonschema)
     }
   }
   ```
2. **Step 17.2**: Write `internal/monitor/monitor_test.go`:

   * Simulate valid JSON → no violation.
   * Simulate invalid JSON (e.g., extra field) → expect recorded violation.
3. **Step 17.3**: Implement `internal/reporting/retrospective.go`:

   ```go
   func GenerateRetrospective(logs []LogEntry) RetrospectiveReport {
     // Scan logs for StepResult failures, schema violations, aggregate counts
   }
   ```
4. **Step 17.4**: Write `internal/reporting/retrospective_test.go`:

   * Provide a slice of fake `LogEntry` (with known failures) → assert `RetrospectiveReport` fields match expectations.

---

### Chunk 18: Final Polish & Documentation

1. **Step 18.1**: Update `README.md` with:

   * Build instructions (`go generate`, `go build`).
   * How to run tests.
   * How to start server.
2. **Step 18.2**: Add package comments (e.g., top of each `internal/…/…go`).
3. **Step 18.3**: Ensure `make ci` passes (generates code, lints, tests).
4. **Step 18.4**: Tag a 1.0.0 release.

---

## 4. Third Round: Final “Right-Sized” Steps

We’ve refined each chunk into small steps. They are now granular enough to implement safely and to write tests before implementation. We can move on to create a series of LLM prompts, each corresponding to one **Step** above. Each prompt will ask the LLM to generate code and tests for that step, ensuring TDD. Prompts are grouped by chunk.

---

## 5. LLM Prompts (with Code Fences)

Below are the prompts, each delimited in a text code fence. Implement them **in order**. After each prompt is executed, the generated code should be committed before moving on to the next.

---

### Chunk 1: Project & CI Setup

```text
**Prompt 1.1**  
You are creating a new Git repository called `payment-orchestrator`. Initialize a Go module with path `github.com/yourorg/payment-orchestrator`. Create the following folder structure:

```

.
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── adapter/
│   ├── orchestrator/
│   ├── planbuilder/
│   ├── policy/
│   ├── processor/
│   ├── router/
│   └── context/
├── pkg/
│   └── gen/
├── schemas/
├── go.mod
└── go.sum

```

Also add a `Makefile` with the targets:
- `gen`: runs `buf generate`.
- `lint`: runs `golangci-lint run`.
- `test`: runs `go test ./...`.

Initialize a GitHub Actions CI configuration in `.github/workflows/ci.yml` that on every push:
1. Checks out code.
2. Installs Buf and runs `make gen`.
3. Installs `golangci-lint` and runs `make lint`.
4. Runs `make test`.

Generate the `go.mod` file and any initial stubs (empty `main.go`, directories). Ensure `go.sum` is created accordingly.  
```

---

### Chunk 2: Schema Definition & Code Generation

````text
**Prompt 2.1**  
Under `schemas/external/v1/`, create a file `external_request.proto` with the following Protobuf definition:

```proto
syntax = "proto3";
package external;

message ExternalRequest {
  string request_id = 1;
  string merchant_id = 2;
  int64 amount = 3;
  string currency = 4;
  CustomerDetails customer = 5;
  PaymentMethod payment_method = 6;
  map<string, string> metadata = 7;
}

message CustomerDetails {
  string name = 1;
  string email = 2;
  string phone = 3;
}

message PaymentMethod {
  oneof method {
    CardInfo card = 1;
    WalletInfo wallet = 2;
  }
}

message CardInfo {
  string number = 1;
  string exp_month = 2;
  string exp_year = 3;
  string cvc = 4;
}

message WalletInfo {
  string wallet_id = 1;
  string wallet_type = 2;
}
````

Under `schemas/internal/v1/`, create three files:

1. **`payment_plan.proto`**:

   ```proto
   syntax = "proto3";
   package internal;

   message PaymentPlan {
     string plan_id = 1;
     repeated PaymentStep steps = 2;
   }
   ```

2. **`payment_step.proto`**:

   ```proto
   syntax = "proto3";
   package internal;

   message PaymentStep {
     string step_id = 1;
     string provider_name = 2;
     int64 amount = 3;
     string currency = 4;
     bool   is_fan_out = 5;
     map<string, string> metadata = 6;
   }
   ```

3. **`step_result.proto`**:

   ```proto
   syntax = "proto3";
   package internal;

   message StepResult {
     string step_id = 1;
     bool   success = 2;
     string provider_name = 3;
     string error_code = 4;
     int64  latency_ms = 5;
     map<string, string> details = 6;
   }

   message PaymentResult {
     string plan_id = 1;
     bool   success = 2;
     repeated StepResult steps = 3;
   }
   ```

Finally, at the project root, create `buf.yaml`:

```yaml
version: v1
name: yourorg/payment-orchestrator
build:
  roots:
    - schemas
```

After that, run `buf generate`. Ensure generated Go code appears under `pkg/gen/` (for both external and internal packages). Commit the generated files.

````

---

### Chunk 3: Context & Dual Boundary

```text
**Prompt 3.1**  
Create `internal/context/trace.go`. Define a `TraceContext` struct:

```go
package context

type TraceContext struct {
  TraceID string
  SpanID  string
  Baggage map[string]string
}

// NewTraceContext returns a new TraceContext with randomly generated IDs.
func NewTraceContext() TraceContext {
  return TraceContext{
    TraceID: uuid.New().String(),
    SpanID:  uuid.New().String(),
    Baggage: make(map[string]string),
  }
}
````

Add any required imports (e.g., `"github.com/google/uuid"`).

````

```text
**Prompt 3.2**  
Create `internal/context/domain.go`. Define `DomainContext` and a constructor:

```go
package context

type DomainContext struct {
  MerchantID            string
  MerchantPolicyVersion int
  UserPreferences       map[string]string
  TimeoutConfig         TimeoutConfig    // define TimeoutConfig elsewhere as needed
  RetryPolicy           RetryPolicy      // define RetryPolicy type accordingly
  FeatureFlags          map[string]bool
  MerchantConfig        MerchantConfig    // define MerchantConfig type (DB/stub)
}

// BuildDomainContext constructs a DomainContext from an ExternalRequest and MerchantConfig.
func BuildDomainContext(
  ext ExternalRequest,
  cfg MerchantConfig,
) DomainContext {
  return DomainContext{
    MerchantID:            ext.MerchantId,
    MerchantPolicyVersion: cfg.PolicyVersion,
    UserPreferences:       cfg.UserPrefs,
    TimeoutConfig:         cfg.TimeoutConfig,
    RetryPolicy:           cfg.RetryPolicy,
    FeatureFlags:          cfg.FeatureFlags,
    MerchantConfig:        cfg,
  }
}
````

Define the `TimeoutConfig`, `RetryPolicy`, and `MerchantConfig` types (stubs with minimal fields).

````

```text
**Prompt 3.3**  
Create `internal/context/step.go`:

```go
package context

type StepExecutionContext struct {
  TraceID             string
  RemainingBudgetMs   int64
  RetryPolicy         RetryPolicy
  ProviderCredentials Credentials // define Credentials struct
  // You can add FraudScore, Region, MerchantTier, etc. later as needed
}

// DeriveStepCtx returns a StepExecutionContext for a given PaymentStep.
func (d DomainContext) DeriveStepCtx(
  step internal.PaymentStep,
  startTimeMs int64,
) StepExecutionContext {
  // Compute elapsed time
  elapsed := time.Now().UnixMilli() - startTimeMs
  remaining := d.TimeoutConfig.OverallBudgetMs - elapsed

  return StepExecutionContext{
    TraceID:             d.TraceID, // if DomainContext holds TraceID or pass separately
    RemainingBudgetMs:   remaining,
    RetryPolicy:         d.RetryPolicy,
    ProviderCredentials: d.MerchantConfig.GetCredentials(step.ProviderName),
  }
}
````

Add necessary imports (e.g., `"time"`, the generated `internal` package).
Write a unit test in `internal/context/builder_test.go` to verify `BuildDomainContext` and `DeriveStepCtx`.

````

```text
**Prompt 3.4**  
Create `internal/context/builder.go`. Implement:

```go
package context

import "github.com/yourorg/payment-orchestrator/pkg/gen/external"

func BuildContexts(
  ext external.ExternalRequest,
  merchantCfg MerchantConfig,
) (TraceContext, DomainContext, error) {
  traceCtx := NewTraceContext()
  domainCtx := BuildDomainContext(ext, merchantCfg)
  return traceCtx, domainCtx, nil
}
````

Write unit tests (`internal/context/builder_test.go`):

* Provide a fake `ExternalRequest`, fake `MerchantConfig`, call `BuildContexts`, and assert fields are copied correctly.

````

---

### Chunk 4: PlanBuilder (Basic) & Composite Stub

```text
**Prompt 4.1**  
Create `internal/planbuilder/planbuilder.go`:

```go
package planbuilder

import (
  "github.com/google/uuid"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

type CompositeConfig struct {
  FeeThreshold int64
  // any other config fields
}

type MerchantConfigRepository interface {
  Get(merchantID string) (MerchantConfig, error)
}

type PlanBuilder struct {
  compositeConfig     CompositeConfig
  merchantConfigRepo  MerchantConfigRepository
}

func NewPlanBuilder(
  repo MerchantConfigRepository,
  cfg CompositeConfig,
) *PlanBuilder {
  return &PlanBuilder{compositeConfig: cfg, merchantConfigRepo: repo}
}

func (pb *PlanBuilder) Build(
  domainCtx context.DomainContext,
) (internal.PaymentPlan, error) {
  // Naive: one step using default provider from merchant config
  provider := domainCtx.MerchantConfig.DefaultProvider
  amount := domainCtx.MerchantConfig.DefaultAmount
  currency := domainCtx.MerchantConfig.DefaultCurrency

  step := internal.PaymentStep{
    StepId:       uuid.New().String(),
    ProviderName: provider,
    Amount:       amount,
    Currency:     currency,
    IsFanOut:     false,
  }

  return internal.PaymentPlan{
    PlanId: uuid.New().String(),
    Steps:  []internal.PaymentStep{step},
  }, nil
}
````

Also define a minimal `MerchantConfig` type in the same package or in `internal/config/`, with fields `DefaultProvider`, `DefaultAmount`, `DefaultCurrency`, and stubs.

````

```text
**Prompt 4.2**  
Create `internal/planbuilder/planbuilder_test.go`:

```go
package planbuilder

import (
  "testing"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/pkg/gen/external"
)

type StubMerchantRepo struct {}

func (s StubMerchantRepo) Get(merchantID string) (MerchantConfig, error) {
  return MerchantConfig{
    DefaultProvider: "stripe",
    DefaultAmount:   100,
    DefaultCurrency: "USD",
    PolicyVersion:   1,
    UserPrefs:       map[string]string{},
    TimeoutConfig:   TimeoutConfig{OverallBudgetMs: 10000},
    RetryPolicy:     RetryPolicy{},
    FeatureFlags:    map[string]bool{},
    MerchantConfig:  MerchantConfig{},
  }, nil
}

func TestPlanBuilder_Build_Naive(t *testing.T) {
  stubRepo := StubMerchantRepo{}
  pb := NewPlanBuilder(stubRepo, CompositeConfig{FeeThreshold: 0})
  extReq := external.ExternalRequest{
    MerchantId: "merchant-123",
    Amount:     100,
    Currency:   "USD",
    // fill other fields minimally
  }
  merchantCfg, _ := stubRepo.Get("merchant-123")
  traceCtx, domainCtx := context.BuildContexts(extReq, merchantCfg)
  plan, err := pb.Build(domainCtx)
  if err != nil {
    t.Fatalf("unexpected error: %v", err)
  }
  if len(plan.Steps) != 1 {
    t.Fatalf("expected 1 step, got %d", len(plan.Steps))
  }
  step := plan.Steps[0]
  if step.ProviderName != "stripe" || step.Amount != 100 || step.Currency != "USD" {
    t.Fatalf("step fields incorrect: %+v", step)
  }
}
````

Run `go test ./internal/planbuilder`.

````

```text
**Prompt 4.3**  
Create `internal/planbuilder/composite.go`:

```go
package planbuilder

import "github.com/yourorg/payment-orchestrator/pkg/gen/internal"

type CompositePaymentService struct {}

func NewCompositePaymentService() *CompositePaymentService {
  return &CompositePaymentService{}
}

// For now, just return the input plan unchanged.
func (c *CompositePaymentService) Optimize(
  rawPlan internal.PaymentPlan,
) internal.PaymentPlan {
  return rawPlan
}
````

Write a test in `internal/planbuilder/composite_test.go`:

```go
package planbuilder

import (
  "testing"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func TestOptimize_Noop(t *testing.T) {
  svc := NewCompositePaymentService()
  raw := internal.PaymentPlan{
    PlanId: "p1",
    Steps: []internal.PaymentStep{
      {StepId: "s1", ProviderName: "stripe", Amount: 100, Currency: "USD"},
    },
  }
  out := svc.Optimize(raw)
  if out.PlanId != raw.PlanId {
    t.Fatalf("expected same PlanId, got %s", out.PlanId)
  }
}
```

````

---

### Chunk 5: PaymentPolicyEnforcer (Basic)

```text
**Prompt 5.1**  
Create `internal/policy/policy.go`:

```go
package policy

import "github.com/yourorg/payment-orchestrator/pkg/gen/internal"

type PolicyDecision struct {
  AllowRetry   bool
  SkipFallback bool
}

type PaymentPolicyEnforcer struct {}

func NewPolicyEnforcer() *PaymentPolicyEnforcer {
  return &PaymentPolicyEnforcer{}
}

func (ppe *PaymentPolicyEnforcer) Evaluate(
  merchantID string,
  step internal.PaymentStep,
  stepCtx context.StepExecutionContext,
) PolicyDecision {
  return PolicyDecision{AllowRetry: true, SkipFallback: false}
}
````

Write `internal/policy/policy_test.go`:

```go
package policy

import (
  "testing"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

func TestEvaluate_Default(t *testing.T) {
  ppe := NewPolicyEnforcer()
  step := internal.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 100, Currency: "USD"}
  stepCtx := context.StepExecutionContext{TraceID: "t1", RemainingBudgetMs: 10000}
  decision := ppe.Evaluate("merchant-123", step, stepCtx)
  if !decision.AllowRetry || decision.SkipFallback {
    t.Fatalf("expected default AllowRetry=true, SkipFallback=false, got %+v", decision)
  }
}
```

````

---

### Chunk 6: Orchestrator (Placeholder ExecuteStep)

```text
**Prompt 6.1**  
Create `internal/orchestrator/orchestrator.go`:

```go
package orchestrator

import (
  "time"

  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/internal/policy"
)

type Orchestrator struct {
  policyEnforcer *policy.PaymentPolicyEnforcer
}

func NewOrchestrator(ppe *policy.PaymentPolicyEnforcer) *Orchestrator {
  return &Orchestrator{policyEnforcer: ppe}
}

func (o *Orchestrator) Execute(
  traceCtx context.TraceContext,
  plan internal.PaymentPlan,
  domainCtx context.DomainContext,
) (internal.PaymentResult, error) {
  var results []internal.StepResult

  for _, step := range plan.Steps {
    startTimeMs := time.Now().UnixMilli()
    stepCtx := domainCtx.DeriveStepCtx(step, startTimeMs)

    // Policy check
    decision := o.policyEnforcer.Evaluate(domainCtx.MerchantID, step, stepCtx)
    if decision.SkipFallback {
      results = append(results, internal.StepResult{
        StepId:       step.StepId,
        Success:      false,
        ProviderName: step.ProviderName,
        ErrorCode:    "PolicySkip",
        LatencyMs:    0,
      })
      continue
    }

    // Placeholder success
    results = append(results, internal.StepResult{
      StepId:       step.StepId,
      Success:      true,
      ProviderName: step.ProviderName,
      ErrorCode:    "",
      LatencyMs:    10,
    })
  }

  return internal.PaymentResult{
    PlanId:  plan.PlanId,
    Success: true,
    Steps:   results,
  }, nil
}
````

Then write `internal/orchestrator/orchestrator_test.go`:

```go
package orchestrator

import (
  "testing"

  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/internal/policy"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func TestExecute_Placeholder(t *testing.T) {
  ppe := policy.NewPolicyEnforcer()
  o := NewOrchestrator(ppe)

  // Build a dummy plan with two steps
  plan := internal.PaymentPlan{
    PlanId: "plan1",
    Steps: []internal.PaymentStep{
      {StepId: "s1", ProviderName: "stripe", Amount: 100, Currency: "USD"},
      {StepId: "s2", ProviderName: "stripe", Amount: 50, Currency: "USD"},
    },
  }
  domainCtx := context.DomainContext{MerchantID: "m1", MerchantConfig: MerchantConfig{/*stub fields*/}}

  result, err := o.Execute(context.NewTraceContext(), plan, domainCtx)
  if err != nil {
    t.Fatalf("unexpected error: %v", err)
  }
  if len(result.Steps) != 2 {
    t.Fatalf("expected 2 step results, got %d", len(result.Steps))
  }
  for _, stepRes := range result.Steps {
    if !stepRes.Success {
      t.Fatalf("expected success=true, got %+v", stepRes)
    }
  }
}
```

````

---

### Chunk 7: ProviderAdapter Interface & Mock Adapter

```text
**Prompt 7.1**  
Create `internal/adapter/adapter.go`:

```go
package adapter

import (
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

type ProviderAdapter interface {
  Process(
    traceCtx context.TraceContext,
    payload internal.ProviderPayload,
    stepCtx context.StepExecutionContext,
  ) (internal.ProviderResult, error)
}
````

````

```text
**Prompt 7.2**  
Create `internal/adapter/mock/mock_adapter.go`:

```go
package mock

import (
  "errors"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

type MockAdapter struct {
  FailFirst bool
  Called    int
}

func NewMockAdapter(failFirst bool) *MockAdapter {
  return &MockAdapter{FailFirst: failFirst, Called: 0}
}

func (m *MockAdapter) Process(
  traceCtx context.TraceContext,
  payload internal.ProviderPayload,
  stepCtx context.StepExecutionContext,
) (internal.ProviderResult, error) {
  m.Called++
  if m.FailFirst && m.Called == 1 {
    return internal.ProviderResult{
      StepId:    payload.StepId,
      Success:   false,
      Provider:  "mock",
      ErrorCode: "TransientError",
      LatencyMs: 5,
    }, errors.New("transient")
  }
  return internal.ProviderResult{
    StepId:    payload.StepId,
    Success:   true,
    Provider:  "mock",
    ErrorCode: "",
    LatencyMs: 5,
  }, nil
}
````

````

```text
**Prompt 7.3**  
Create `internal/adapter/mock/mock_adapter_test.go`:

```go
package mock

import (
  "testing"

  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

func TestMockAdapter_FailFirst(t *testing.T) {
  adapter := NewMockAdapter(true)
  payload := internal.ProviderPayload{StepId: "s1"}
  stepCtx := context.StepExecutionContext{TraceID: "t1", RemainingBudgetMs: 1000}

  // First call should fail
  res1, err1 := adapter.Process(context.NewTraceContext(), payload, stepCtx)
  if err1 == nil || res1.Success {
    t.Fatalf("expected first call to fail, got success=%v, err=%v", res1.Success, err1)
  }

  // Second call should succeed
  res2, err2 := adapter.Process(context.NewTraceContext(), payload, stepCtx)
  if err2 != nil || !res2.Success {
    t.Fatalf("expected second call to succeed, got success=%v, err=%v", res2.Success, err2)
  }
}
````

````

---

### Chunk 8: Processor Layer

```text
**Prompt 8.1**  
Create `internal/processor/processor.go`:

```go
package processor

import (
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/internal/adapter"
)

type Processor struct {
  adapters map[string]adapter.ProviderAdapter
}

func NewProcessor(adapters map[string]adapter.ProviderAdapter) *Processor {
  return &Processor{adapters: adapters}
}

func (p *Processor) ProcessSingleStep(
  traceCtx context.TraceContext,
  step internal.PaymentStep,
  stepCtx context.StepExecutionContext,
) (internal.StepResult, error) {
  adapterImpl, ok := p.adapters[step.ProviderName]
  if !ok {
    return internal.StepResult{
      StepId:       step.StepId,
      Success:      false,
      ProviderName: step.ProviderName,
      ErrorCode:    "AdapterNotFound",
      LatencyMs:    0,
      Details:      map[string]string{"info": "No adapter registered"},
    }, nil
  }

  providerRes, err := adapterImpl.Process(traceCtx, step.Payload, stepCtx)
  if err != nil {
    return internal.StepResult{
      StepId:       step.StepId,
      Success:      false,
      ProviderName: step.ProviderName,
      ErrorCode:    providerRes.ErrorCode,
      LatencyMs:    providerRes.LatencyMs,
      Details:      providerRes.Details,
    }, err
  }

  return internal.StepResult{
    StepId:       step.StepId,
    Success:      providerRes.Success,
    ProviderName: providerRes.Provider,
    ErrorCode:    providerRes.ErrorCode,
    LatencyMs:    providerRes.LatencyMs,
    Details:      providerRes.Details,
  }, nil
}
````

````

```text
**Prompt 8.2**  
Create `internal/processor/processor_test.go`:

```go
package processor

import (
  "errors"
  "testing"

  "github.com/yourorg/payment-orchestrator/internal/adapter/mock"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func TestProcessSingleStep_Success(t *testing.T) {
  // MockAdapter that always succeeds
  mockAdapter := mock.NewMockAdapter(false)
  proc := NewProcessor(map[string]adapter.ProviderAdapter{"mock": mockAdapter})

  step := internal.PaymentStep{StepId: "s1", ProviderName: "mock", Amount: 100, Currency: "USD"}
  stepCtx := context.StepExecutionContext{TraceID: "t1", RemainingBudgetMs: 1000}

  res, err := proc.ProcessSingleStep(context.NewTraceContext(), step, stepCtx)
  if err != nil || !res.Success {
    t.Fatalf("expected success, got res=%+v, err=%v", res, err)
  }
}

func TestProcessSingleStep_AdapterError(t *testing.T) {
  // MockAdapter that fails first then succeeds
  mockAdapter := mock.NewMockAdapter(true)
  proc := NewProcessor(map[string]adapter.ProviderAdapter{"mock": mockAdapter})

  step := internal.PaymentStep{StepId: "s1", ProviderName: "mock", Amount: 100, Currency: "USD"}
  stepCtx := context.StepExecutionContext{TraceID: "t1", RemainingBudgetMs: 1000}

  // First call: error
  res1, err1 := proc.ProcessSingleStep(context.NewTraceContext(), step, stepCtx)
  if err1 == nil || res1.Success {
    t.Fatalf("expected first call to error, got res=%+v, err=%v", res1, err1)
  }

  // Second call: success
  res2, err2 := proc.ProcessSingleStep(context.NewTraceContext(), step, stepCtx)
  if err2 != nil || !res2.Success {
    t.Fatalf("expected second call to succeed, got res=%+v, err=%v", res2, err2)
  }
}
````

````

---

### Chunk 9: Router (Minimal Fallback)

```text
**Prompt 9.1**  
Create `internal/router/router.go`:

```go
package router

import (
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/internal/adapter"
)

type Router struct {
  adapters       map[string]adapter.ProviderAdapter
  fallbackChains map[string][]string // merchantID → fallback chain
}

func NewRouter(
  adapters map[string]adapter.ProviderAdapter,
  chains map[string][]string,
) *Router {
  return &Router{adapters: adapters, fallbackChains: chains}
}

func (r *Router) ExecuteStep(
  traceCtx context.TraceContext,
  step internal.PaymentStep,
  stepCtx context.StepExecutionContext,
) internal.StepResult {
  primary := step.ProviderName
  if adapterImpl, ok := r.adapters[primary]; ok {
    res, err := adapterImpl.Process(traceCtx, step.Payload, stepCtx)
    if err == nil && res.Success {
      return internal.StepResult{
        StepId:       step.StepId,
        Success:      true,
        ProviderName: primary,
        ErrorCode:    "",
        LatencyMs:    res.LatencyMs,
      }
    }
  }

  chain := r.fallbackChains[stepCtx.MerchantID]
  for _, fb := range chain {
    if fb == primary {
      continue
    }
    if adapterImpl, ok := r.adapters[fb]; ok {
      res, err := adapterImpl.Process(traceCtx, step.Payload, stepCtx)
      if err == nil && res.Success {
        return internal.StepResult{
          StepId:       step.StepId,
          Success:      true,
          ProviderName: fb,
          ErrorCode:    "",
          LatencyMs:    res.LatencyMs,
        }
      }
    }
  }

  return internal.StepResult{
    StepId:       step.StepId,
    Success:      false,
    ProviderName: primary,
    ErrorCode:    "AllProvidersFailed",
    LatencyMs:    0,
  }
}
````

Create `internal/router/router_test.go`:

```go
package router

import (
  "testing"

  "github.com/yourorg/payment-orchestrator/internal/adapter/mock"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func TestExecuteStep_Fallback(t *testing.T) {
  // Primary fails, fallback succeeds
  adapters := map[string]adapter.ProviderAdapter{
    "stripe": mock.NewMockAdapter(true),
    "adyen":  mock.NewMockAdapter(false),
  }
  chains := map[string][]string{"merchant-123": {"stripe", "adyen"}}
  r := NewRouter(adapters, chains)

  step := internal.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 100, Currency: "USD"}
  stepCtx := context.StepExecutionContext{MerchantID: "merchant-123", TraceID: "t1", RemainingBudgetMs: 1000}

  res := r.ExecuteStep(context.NewTraceContext(), step, stepCtx)
  if !res.Success || res.ProviderName != "adyen" {
    t.Fatalf("expected fallback to adyen, got %+v", res)
  }
}
```

````

---

### Chunk 10: Orchestrator (Integrate Router & Policy)

```text
**Prompt 10.1**  
Update `internal/orchestrator/orchestrator.go` to integrate `Router`. Change:

```go
type Orchestrator struct {
  policyEnforcer *policy.PaymentPolicyEnforcer
  router         *router.Router
}

func NewOrchestrator(
  ppe *policy.PaymentPolicyEnforcer,
  r *router.Router,
) *Orchestrator {
  return &Orchestrator{policyEnforcer: ppe, router: r}
}

func (o *Orchestrator) Execute(... ) ... {
  // Inside loop, replace placeholder with:
  stepResult := o.router.ExecuteStep(traceCtx, step, stepCtx)
  if !stepResult.Success && !decision.AllowRetry {
    return internal.PaymentResult{PlanId: plan.PlanId, Success: false, Steps: append(results, stepResult)}, nil
  }
  results = append(results, stepResult)
}
````

Ensure imports are updated (`"github.com/yourorg/payment-orchestrator/internal/router"`).

````

```text
**Prompt 10.2**  
Create `internal/orchestrator/orchestrator_integration_test.go`:

```go
package orchestrator

import (
  "testing"

  "github.com/yourorg/payment-orchestrator/internal/adapter/mock"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/internal/orchestrator"
  "github.com/yourorg/payment-orchestrator/internal/policy"
  "github.com/yourorg/payment-orchestrator/internal/router"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func TestOrchestrator_FallbackAndPolicy(t *testing.T) {
  // Setup mock adapters: stripe fails, adyen succeeds
  adapters := map[string]adapter.ProviderAdapter{
    "stripe": mock.NewMockAdapter(true),
    "adyen":  mock.NewMockAdapter(false),
  }
  chains := map[string][]string{"merchant-123": {"stripe", "adyen"}}
  r := router.NewRouter(adapters, chains)
  ppe := policy.NewPolicyEnforcer()
  o := NewOrchestrator(ppe, r)

  plan := internal.PaymentPlan{
    PlanId: "plan1",
    Steps: []internal.PaymentStep{
      {StepId: "s1", ProviderName: "stripe", Amount: 100, Currency: "USD"},
      {StepId: "s2", ProviderName: "adyen", Amount: 50, Currency: "USD"},
    },
  }

  domainCtx := context.DomainContext{MerchantID: "merchant-123", MerchantConfig: MerchantConfig{/*stub*/}}

  result, err := o.Execute(context.NewTraceContext(), plan, domainCtx)
  if err != nil {
    t.Fatalf("unexpected error: %v", err)
  }
  if !result.Success || len(result.Steps) != 2 {
    t.Fatalf("expected 2 step results with success, got %+v", result)
  }
  if result.Steps[0].ProviderName != "adyen" {
    t.Fatalf("expected fallback for first step, got %s", result.Steps[0].ProviderName)
  }
}
````

````

---

### Chunk 11: StripeAdapter (Simple Implementation)

```text
**Prompt 11.1**  
Create `internal/adapter/stripe/stripe_adapter.go`:

```go
package stripe

import (
  "bytes"
  "errors"
  "io/ioutil"
  "net/http"
  "time"

  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

type StripeAdapter struct {
  httpClient *http.Client
  apiKey     string
}

func NewStripeAdapter(apiKey string) *StripeAdapter {
  return &StripeAdapter{
    httpClient: &http.Client{Timeout: 2 * time.Second},
    apiKey:     apiKey,
  }
}

func (s *StripeAdapter) Process(
  traceCtx context.TraceContext,
  payload internal.ProviderPayload,
  stepCtx context.StepExecutionContext,
) (internal.ProviderResult, error) {
  reqBody := buildStripePayload(payload, stepCtx.ProviderCredentials)
  req, _ := http.NewRequest("POST", "https://api.stripe.com/v1/charges", bytes.NewBuffer(reqBody))
  req.Header.Set("Authorization", "Bearer "+s.apiKey)
  req.Header.Set("Idempotency-Key", generateIdempotencyKey(traceCtx.TraceID, payload.StepId))

  var lastErr error
  for attempt := 0; attempt < 3; attempt++ {
    start := time.Now()
    resp, err := s.httpClient.Do(req)
    latency := time.Since(start).Milliseconds()

    if err != nil {
      lastErr = err
      time.Sleep(time.Duration(attempt+1) * time.Second)
      continue
    }
    defer resp.Body.Close()

    body, _ := ioutil.ReadAll(resp.Body)
    if resp.StatusCode == 200 {
      return internal.ProviderResult{
        StepId:      payload.StepId,
        Success:     true,
        Provider:    "stripe",
        ErrorCode:   "",
        LatencyMs:   latency,
        RawResponse: body,
      }, nil
    } else if resp.StatusCode >= 500 || resp.StatusCode == 429 {
      lastErr = errors.New("transient stripe error")
      time.Sleep(time.Duration(attempt+1) * time.Second)
      continue
    } else {
      errCode := parseStripeErrorCode(body)
      return internal.ProviderResult{
        StepId:      payload.StepId,
        Success:     false,
        Provider:    "stripe",
        ErrorCode:   errCode,
        LatencyMs:   latency,
        RawResponse: body,
      }, nil
    }
  }
  return internal.ProviderResult{StepId: payload.StepId, Success: false, Provider: "stripe", ErrorCode: "MaxRetriesExceeded"}, lastErr
}
````

Implement helper functions `buildStripePayload(...)` and `generateIdempotencyKey(...)`.

````

```text
**Prompt 11.2**  
Create `internal/adapter/stripe/stripe_adapter_test.go`:

```go
package stripe

import (
  "net/http"
  "net/http/httptest"
  "testing"

  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

func TestStripeAdapter_RetrySuccess(t *testing.T) {
  // httptest server that fails once then succeeds
  callCount := 0
  ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    callCount++
    if callCount == 1 {
      w.WriteHeader(500)
      w.Write([]byte(`{"error":"server"}`))
    } else {
      w.WriteHeader(200)
      w.Write([]byte(`{"id":"ch_123","status":"succeeded"}`))
    }
  }))
  defer ts.Close()

  adapter := &StripeAdapter{
    httpClient: &http.Client{Timeout: 2 * time.Second},
    apiKey:     "testkey",
  }
  // Override endpoint to ts.URL
  stripeAPIURL = ts.URL

  stepCtx := context.StepExecutionContext{TraceID: "t1", RemainingBudgetMs: 10000}
  payload := internal.ProviderPayload{StepId: "s1", /* fill other fields if needed */ }

  res, err := adapter.Process(context.NewTraceContext(), payload, stepCtx)
  if err != nil {
    t.Fatalf("expected no error, got %v", err)
  }
  if !res.Success {
    t.Fatalf("expected success=true, got %+v", res)
  }
  if callCount != 2 {
    t.Fatalf("expected 2 calls, got %d", callCount)
  }
}
````

````

---

### Chunk 12: Router (Full Features)

```text
**Prompt 12.1**  
Create `internal/router/circuitbreaker/circuitbreaker.go`:

```go
package circuitbreaker

import "time"

type CircuitBreakerService struct {
  unhealthyProviders map[string]time.Time
  cooldown           time.Duration
}

func NewCircuitBreaker(cooldown time.Duration) *CircuitBreakerService {
  return &CircuitBreakerService{unhealthyProviders: make(map[string]time.Time), cooldown: cooldown}
}

func (cb *CircuitBreakerService) MarkUnhealthy(provider string) {
  cb.unhealthyProviders[provider] = time.Now().Add(cb.cooldown)
}

func (cb *CircuitBreakerService) IsHealthy(provider string) bool {
  until, exists := cb.unhealthyProviders[provider]
  if !exists || time.Now().After(until) {
    return true
  }
  return false
}
````

Create `internal/router/circuitbreaker/circuitbreaker_test.go`:

```go
package circuitbreaker

import (
  "testing"
  "time"
)

func TestCircuitBreaker(t *testing.T) {
  cb := NewCircuitBreaker(100 * time.Millisecond)
  if !cb.IsHealthy("p1") {
    t.Fatal("expected healthy initially")
  }
  cb.MarkUnhealthy("p1")
  if cb.IsHealthy("p1") {
    t.Fatal("expected unhealthy immediately after marking")
  }
  time.Sleep(150 * time.Millisecond)
  if !cb.IsHealthy("p1") {
    t.Fatal("expected healthy after cooldown")
  }
}
```

````

```text
**Prompt 12.2**  
Update `internal/router/router.go` to integrate `CircuitBreakerService` and SLA enforcement:

```go
package router

import (
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/internal/adapter"
  "github.com/yourorg/payment-orchestrator/internal/router/circuitbreaker"
)

type Router struct {
  adapters        map[string]adapter.ProviderAdapter
  fallbackChains  map[string][]string
  circuitBreaker  *circuitbreaker.CircuitBreakerService
}

func NewRouter(
  adapters map[string]adapter.ProviderAdapter,
  chains map[string][]string,
  cb *circuitbreaker.CircuitBreakerService,
) *Router {
  return &Router{adapters: adapters, fallbackChains: chains, circuitBreaker: cb}
}

func (r *Router) ExecuteStep(
  traceCtx context.TraceContext,
  step internal.PaymentStep,
  stepCtx context.StepExecutionContext,
) internal.StepResult {
  primary := step.ProviderName

  // Circuit-breaker check
  if !r.circuitBreaker.IsHealthy(primary) {
    return r.tryFallback(traceCtx, step, stepCtx, primary)
  }

  // SLA enforcement
  if stepCtx.RemainingBudgetMs < minimumAdapterTimeoutMs {
    return internal.StepResult{
      StepId:       step.StepId,
      Success:      false,
      ProviderName: primary,
      ErrorCode:    "TimeoutBudgetExceeded",
      LatencyMs:    0,
    }
  }

  // Primary attempt
  res, err := r.adapters[primary].Process(traceCtx, step.Payload, stepCtx)
  if err != nil || !res.Success {
    // Mark primary as unhealthy
    r.circuitBreaker.MarkUnhealthy(primary)
    return r.tryFallback(traceCtx, step, stepCtx, primary)
  }

  // Success
  return internal.StepResult{
    StepId:       step.StepId,
    Success:      true,
    ProviderName: primary,
    ErrorCode:    "",
    LatencyMs:    res.LatencyMs,
  }
}

func (r *Router) tryFallback(
  traceCtx context.TraceContext,
  step internal.PaymentStep,
  stepCtx context.StepExecutionContext,
  failedProvider string,
) internal.StepResult {
  chain := r.fallbackChains[stepCtx.MerchantID]
  for _, fb := range chain {
    if fb == failedProvider {
      continue
    }
    if !r.circuitBreaker.IsHealthy(fb) {
      continue
    }
    res, err := r.adapters[fb].Process(traceCtx, step.Payload, stepCtx)
    if err == nil && res.Success {
      return internal.StepResult{
        StepId:       step.StepId,
        Success:      true,
        ProviderName: fb,
        ErrorCode:    "",
        LatencyMs:    res.LatencyMs,
      }
    }
    // Mark fallback as unhealthy if it fails
    r.circuitBreaker.MarkUnhealthy(fb)
  }
  return internal.StepResult{
    StepId:       step.StepId,
    Success:      false,
    ProviderName: failedProvider,
    ErrorCode:    "AllProvidersFailed",
    LatencyMs:    0,
  }
}
````

Create `internal/router/router_full_test.go` to cover:

* Primary fails, fallback marked unhealthy, next fallback succeeds.
* SLA budget expired: `stepCtx.RemainingBudgetMs=0` → returns `TimeoutBudgetExceeded`.
* Circuit breaker causes skip for a provider still in cooldown.

````

---

### Chunk 13: CompositePaymentService (Full Optimizer)

```text
**Prompt 13.1**  
Create `internal/planbuilder/optimizer.go` with a simple optimization:

```go
package planbuilder

import (
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func (c *CompositePaymentService) Optimize(
  rawPlan internal.PaymentPlan,
) internal.PaymentPlan {
  // For now, if there are two providers A and B in metadata, split equally.
  if len(rawPlan.Steps) == 1 && rawPlan.Steps[0].Metadata["split"] == "equal" {
    amount := rawPlan.Steps[0].Amount / 2
    return internal.PaymentPlan{
      PlanId: rawPlan.PlanId,
      Steps: []internal.PaymentStep{
        {
          StepId:       rawPlan.Steps[0].StepId + "-A",
          ProviderName: "A",
          Amount:       amount,
          Currency:     rawPlan.Steps[0].Currency,
          IsFanOut:     false,
        },
        {
          StepId:       rawPlan.Steps[0].StepId + "-B",
          ProviderName: "B",
          Amount:       amount,
          Currency:     rawPlan.Steps[0].Currency,
          IsFanOut:     false,
        },
      },
    }
  }
  return rawPlan
}
````

Then write `internal/planbuilder/optimizer_test.go`:

```go
package planbuilder

import (
  "testing"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func TestOptimize_EqualSplit(t *testing.T) {
  svc := NewCompositePaymentService()
  raw := internal.PaymentPlan{
    PlanId: "p1",
    Steps: []internal.PaymentStep{
      {
        StepId:       "s1",
        ProviderName: "unused",
        Amount:       100,
        Currency:     "USD",
        Metadata:     map[string]string{"split": "equal"},
      },
    },
  }
  out := svc.Optimize(raw)
  if len(out.Steps) != 2 {
    t.Fatalf("expected 2 steps, got %d", len(out.Steps))
  }
  if out.Steps[0].Amount != 50 || out.Steps[1].Amount != 50 {
    t.Fatalf("expected equal split, got %+v", out.Steps)
  }
}
```

````

---

### Chunk 14: PaymentPolicyEnforcer (Full DSL)

```text
**Prompt 14.1**  
Update `internal/policy/policy.go` to import `github.com/Knetic/govaluate` and implement DSL evaluation:

```go
package policy

import (
  "github.com/Knetic/govaluate"
  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

type PaymentPolicyEnforcer struct {
  expressionCache []*govaluate.EvaluableExpression
}

func NewPolicyEnforcer(rules []string) (*PaymentPolicyEnforcer, error) {
  cache := make([]*govaluate.EvaluableExpression, 0, len(rules))
  for _, exprStr := range rules {
    expr, err := govaluate.NewEvaluableExpression(exprStr)
    if err != nil {
      return nil, err
    }
    cache = append(cache, expr)
  }
  return &PaymentPolicyEnforcer{expressionCache: cache}, nil
}

func (ppe *PaymentPolicyEnforcer) Evaluate(
  merchantID string,
  step internal.PaymentStep,
  stepCtx context.StepExecutionContext,
) PolicyDecision {
  vars := map[string]interface{}{
    "fraudScore":  stepCtx.FraudScore,   // assume FraudScore is set in StepExecutionContext
    "region":      stepCtx.Region,       // assume Region is set
    "amount":      step.Amount,
    "merchantTier": stepCtx.MerchantTier, // assume MerchantTier set
  }
  for _, expr := range ppe.expressionCache {
    result, err := expr.Evaluate(vars)
    if err != nil {
      continue
    }
    matched, ok := result.(bool)
    if ok && matched {
      return PolicyDecision{AllowRetry: false, SkipFallback: true}
    }
  }
  return PolicyDecision{AllowRetry: true, SkipFallback: false}
}
````

Then create `internal/policy/policy_test.go`:

```go
package policy

import (
  "testing"

  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
  "github.com/yourorg/payment-orchestrator/internal/context"
)

func TestEvaluate_DSL(t *testing.T) {
  rules := []string{"fraudScore > 0.8"}
  ppe, err := NewPolicyEnforcer(rules)
  if err != nil {
    t.Fatalf("unexpected error: %v", err)
  }

  step := internal.PaymentStep{StepId: "s1", ProviderName: "stripe", Amount: 100, Currency: "USD"}
  stepCtxHigh := context.StepExecutionContext{FraudScore: 0.9, Region: "US", RequestAmount: 100}
  decision := ppe.Evaluate("m1", step, stepCtxHigh)
  if !decision.SkipFallback {
    t.Fatalf("expected SkipFallback for fraudScore>0.8, got %+v", decision)
  }

  stepCtxLow := context.StepExecutionContext{FraudScore: 0.5, Region: "US", RequestAmount: 100}
  decision2 := ppe.Evaluate("m1", step, stepCtxLow)
  if !decision2.AllowRetry || decision2.SkipFallback {
    t.Fatalf("expected default AllowRetry=true, got %+v", decision2)
  }
}
```

````

---

### Chunk 15: API Layer & Validation

```text
**Prompt 15.1**  
Create `cmd/server/main.go`:

```go
package main

import (
  "net/http"

  "github.com/gin-gonic/gin"
  external "github.com/yourorg/payment-orchestrator/pkg/gen/external"
  "github.com/yourorg/payment-orchestrator/internal/context"
  "github.com/yourorg/payment-orchestrator/internal/orchestrator"
  "github.com/yourorg/payment-orchestrator/internal/planbuilder"
  "github.com/yourorg/payment-orchestrator/internal/policy"
  "github.com/yourorg/payment-orchestrator/internal/router"
)

func processPaymentHandler(c *gin.Context) {
  var extReq external.ExternalRequest
  if err := c.ShouldBindJSON(&extReq); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }

  // Fetch merchant configuration from a stub or service:
  merchantCfg := MerchantConfig{/* stub fields */}

  traceCtx, domainCtx := context.BuildContexts(extReq, merchantCfg)

  // Build plan
  planBuilder := planbuilder.NewPlanBuilder(&StubMerchantRepo{}, CompositeConfig{FeeThreshold: 0})
  plan, err := planBuilder.Build(domainCtx)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
  }

  // Set up policy & router
  rules := []string{"fraudScore > 0.8"}
  ppe, _ := policy.NewPolicyEnforcer(rules)
  adapterMap := map[string]adapter.ProviderAdapter{"mock": mock.NewMockAdapter(false)}
  fallbackChains := map[string][]string{} // stub
  cb := circuitbreaker.NewCircuitBreaker(5 * time.Second)
  r := router.NewRouter(adapterMap, fallbackChains, cb)
  orchestrator := orchestrator.NewOrchestrator(ppe, r)

  result, err := orchestrator.Execute(traceCtx, plan, domainCtx)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
  }

  c.JSON(http.StatusOK, result)
}

func main() {
  r := gin.Default()
  r.POST("/process-payment", processPaymentHandler)
  r.Run() // listen :8080
}
````

Write `cmd/server/server_test.go` using `httptest`:

```go
package main

import (
  "bytes"
  "net/http"
  "net/http/httptest"
  "testing"

  "github.com/gin-gonic/gin"
  "github.com/yourorg/payment-orchestrator/pkg/gen/external"
)

func TestProcessPaymentHandler_Valid(t *testing.T) {
  router := gin.Default()
  router.POST("/process-payment", processPaymentHandler)

  reqBody := `{"request_id":"r1","merchant_id":"m1","amount":100,"currency":"USD"}`
  req := httptest.NewRequest("POST", "/process-payment", bytes.NewBufferString(reqBody))
  req.Header.Set("Content-Type", "application/json")

  w := httptest.NewRecorder()
  router.ServeHTTP(w, req)

  if w.Code != http.StatusOK {
    t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
  }
}

func TestProcessPaymentHandler_Invalid(t *testing.T) {
  router := gin.Default()
  router.POST("/process-payment", processPaymentHandler)

  reqBody := `{"amount":100}` // missing required fields
  req := httptest.NewRequest("POST", "/process-payment", bytes.NewBufferString(reqBody))
  req.Header.Set("Content-Type", "application/json")

  w := httptest.NewRecorder()
  router.ServeHTTP(w, req)

  if w.Code != http.StatusBadRequest {
    t.Fatalf("expected 400, got %d", w.Code)
  }
}
```

````

---

### Chunk 16: Observability Integration

```text
**Prompt 16.1**  
Add OpenTelemetry to the Gin router in `cmd/server/main.go`. At the top of `main()`, initialize:

```go
import (
  "go.opentelemetry.io/otel"
  "go.opentelemetry.io/otel/sdk/trace"
  "github.com/yourorg/gin-otel" // assume you have a Gin OTEL middleware package
)

// In main():
tp := trace.NewTracerProvider()
otel.SetTracerProvider(tp)
r := gin.Default()
r.Use(ginotel.Middleware(otel.GetTracerProvider().Tracer("payment-orchestrator")))
````

Ensure spans are created in `PlanBuilder.Build()`, `Orchestrator.Execute()`, and `Router.ExecuteStep()`. For example, wrap `PlanBuilder.Build()` with:

```go
ctx, span := otel.Tracer("payment-orchestrator").Start(context.Background(), "PlanBuilder.Build")
defer span.End()
// existing logic
```

````

```text
**Prompt 16.2**  
Add Prometheus instrumentation in `internal/planbuilder/planbuilder.go`:

1. Import `"github.com/prometheus/client_golang/prometheus"`.
2. At package init, register:
   ```go
   var (
     planRequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
       Name: "plan_requests_total",
       Help: "Total number of plan build requests",
     })
     planBuildDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
       Name:    "plan_build_duration_seconds",
       Help:    "Histogram of plan build durations",
       Buckets: prometheus.DefBuckets,
     })
   )

   func init() {
     prometheus.MustRegister(planRequestsTotal, planBuildDuration)
   }
````

3. In `Build()`, wrap:

   ```go
   planRequestsTotal.Inc()
   timer := prometheus.NewTimer(planBuildDuration)
   defer timer.ObserveDuration()
   // existing build logic
   ```
4. Write `internal/planbuilder/observability_test.go`:

   * Call `PlanBuilder.Build(...)` and inspect `planRequestsTotal` value is incremented by 1.

````

---

### Chunk 17: ContractMonitor & RetrospectiveReporter

```text
**Prompt 17.1**  
Create `internal/monitor/monitor.go`:

```go
package monitor

import (
  "encoding/json"
  "github.com/xeipuuv/gojsonschema"
  "log"
)

func StartContractMonitor(
  schemaFile string,
  msgChan <-chan []byte,
) {
  loader := gojsonschema.NewReferenceLoader("file://" + schemaFile)
  for raw := range msgChan {
    documentLoader := gojsonschema.NewBytesLoader(raw)
    result, err := gojsonschema.Validate(loader, documentLoader)
    if err != nil {
      log.Printf("schema validation error: %v", err)
      continue
    }
    if !result.Valid() {
      for _, desc := range result.Errors() {
        log.Printf("schema violation: %s", desc)
      }
    }
  }
}
````

Write `internal/monitor/monitor_test.go`:

```go
package monitor

import (
  "testing"
)

func TestMonitor_InvalidJSON(t *testing.T) {
  schemaFile := "schemas/external/v1/external_request.json"
  msgChan := make(chan []byte, 1)
  msgChan <- []byte(`{"invalid_field": "xyz"}`)
  close(msgChan)

  // Redirect log output to buffer (omitted for brevity)
  StartContractMonitor(schemaFile, msgChan)
  // Assert that a "schema violation" message was logged
}
```

````

```text
**Prompt 17.2**  
Create `internal/reporting/retrospective.go`:

```go
package reporting

import "github.com/yourorg/payment-orchestrator/pkg/gen/internal"

type LogEntry struct {
  StepResult internal.StepResult
  // add fields for schema violations if desired
}

type RetrospectiveReport struct {
  Date             string
  TotalRequests    int
  TotalFailures    int
  TopFailureReasons []struct {
    Reason string
    Count  int
  }
  SchemaViolations []struct {
    Schema      string
    Count       int
    Examples    []struct{ PayloadID, Field, Description string }
  }
}

func GenerateRetrospective(logs []LogEntry) RetrospectiveReport {
  report := RetrospectiveReport{
    Date:          time.Now().Format("2006-01-02"),
    TotalRequests: len(logs),
  }
  failureCount := 0
  reasonCounts := make(map[string]int)
  for _, entry := range logs {
    if !entry.StepResult.Success {
      failureCount++
      reasonCounts[entry.StepResult.ErrorCode]++
    }
  }
  report.TotalFailures = failureCount
  for reason, cnt := range reasonCounts {
    report.TopFailureReasons = append(report.TopFailureReasons, struct {
      Reason string
      Count  int
    }{Reason: reason, Count: cnt})
  }
  return report
}
````

Write `internal/reporting/retrospective_test.go`:

```go
package reporting

import (
  "testing"

  "github.com/yourorg/payment-orchestrator/pkg/gen/internal"
)

func TestGenerateRetrospective(t *testing.T) {
  logs := []LogEntry{
    {StepResult: internal.StepResult{StepId: "s1", Success: false, ErrorCode: "E1"}},
    {StepResult: internal.StepResult{StepId: "s2", Success: true}},
    {StepResult: internal.StepResult{StepId: "s3", Success: false, ErrorCode: "E1"}},
  }
  report := GenerateRetrospective(logs)
  if report.TotalRequests != 3 || report.TotalFailures != 2 {
    t.Fatalf("expected 3 requests, 2 failures, got %+v", report)
  }
  if len(report.TopFailureReasons) != 1 || report.TopFailureReasons[0].Reason != "E1" {
    t.Fatalf("unexpected top failure reasons: %+v", report.TopFailureReasons)
  }
}
```

````

---

### Chunk 18: Final Polish & Documentation

```text
**Prompt 18.1**  
Update `README.md` with sections:

1. **Project Overview**: Brief description referencing `spec.md`.  
2. **Prerequisites**: Go ≥ 1.18, Buf CLI, `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`.  
3. **Building & Code Generation**:  
````

make gen    # runs buf generate
make build  # go build ./cmd/server

```
4. **Running Tests**:  
```

make test   # runs all unit tests

```
5. **Starting the Server**:
```

go run cmd/server/main.go

```
6. **API Usage Example**:  
```

curl -X POST [http://localhost:8080/process-payment](http://localhost:8080/process-payment)&#x20;
-H "Content-Type: application/json"&#x20;
-d '{"request\_id":"r1","merchant\_id":"m1","amount":100,"currency":"USD"}'

```

Commit all changes and ensure CI passes.  
```

---

## 6. Summary

* **Blueprint**: Outlined the entire system from project setup through final observability and reporting.
* **Chunks**: Eightteen focused chunks grouping related functionality.
* **Small Steps**: Each chunk broken into granular, test-driven steps.
* **LLM Prompts**: A sequence of prompts (tagged as code) instructing code generation + tests for each step.

Follow these prompts in order to generate a fully working, modular payment orchestration system with comprehensive tests, CI, and observability.
