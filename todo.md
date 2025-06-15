```markdown
# TODO List

This `todo.md` tracks which implementation steps have been completed (☑) and which are still pending (☐). Update the checkboxes as you go.

---

## Chunk 1: Project & CI Setup

1. **Repository Initialization**  
   - [☑] Create new Git repository `payment-orchestrator`.
   - [☑] Initialize Go module: `go mod init github.com/yourorg/payment-orchestrator`.
   - [☑] Create folder structure (`cmd/server/main.go`, `internal/...`, `pkg/gen/`, `schemas/`).  *(Note: `schemas/` is `protos/`)*
   - [☑] Add `Makefile` with targets: `gen`, `lint`, `test`.
   - [☑] Add GitHub Actions CI at `.github/workflows/ci.yml` to run `make gen`, `make lint`, `make test`.

---

## Chunk 2: Schema Definition & Code Generation

2. **External Schema**  
   - [☑] Create `schemas/external/v1/external_request.proto`. *(Note: Path is `protos/orchestratorexternalv1/external_request.proto`)*

3. **Internal Schemas**  
   - [☑] Create `schemas/internal/v1/payment_plan.proto`. *(Note: Path is `protos/orchestratorinternalv1/payment_plan.proto`)*
   - [☑] Create `schemas/internal/v1/payment_step.proto`. *(Note: Path is `protos/orchestratorinternalv1/payment_step.proto`)*
   - [☑] Create `schemas/internal/v1/step_result.proto`. *(Note: Path is `protos/orchestratorinternalv1/step_result.proto`)*

4. **Buf Configuration**  
   - [☑] Add `buf.yaml` at project root.
   - [☑] Run `buf generate` and verify generated Go code under `pkg/gen/`.

---

## Chunk 3: Context & Dual Boundary

5. **TraceContext**  
   - [☑] Create `internal/context/trace.go` with `TraceContext` and `NewTraceContext()`.

6. **DomainContext**  
   - [☑] Create `internal/context/domain.go` with `DomainContext` and `BuildDomainContext()`.
   - [☑] Define stubs for `TimeoutConfig`, `RetryPolicy`, `MerchantConfig`.

7. **StepExecutionContext**  
   - [☑] Create `internal/context/step.go` with `StepExecutionContext` and `DeriveStepCtx()`.

8. **Context Builder**  
   - [☑] Create `internal/context/builder.go` with `BuildContexts(ext, merchantCfg)`.
   - [☑] Write tests in `internal/context/builder_test.go`.

---

## Chunk 4: PlanBuilder (Basic) & Composite Stub

9. **PlanBuilder (Naive)**  
   - [☑] Create `internal/planbuilder/planbuilder.go` with `PlanBuilder.Build()` returning a single-step plan.
   - [☑] Define `MerchantConfig` type (with `DefaultProvider`, `DefaultAmount`, `DefaultCurrency`). *(Note: `DefaultAmount` sourced from request)*
   - [☑] Write tests in `internal/planbuilder/planbuilder_test.go`.

10. **CompositePaymentService (Stub)**  
    - [☑] Create `internal/planbuilder/composite.go` with `CompositePaymentService.Optimize()` returning input plan.
    - [☑] Write tests in `internal/planbuilder/composite_test.go`.
    - [☑] Integrate `Optimize` call into `PlanBuilder.Build()`.

---

## Chunk 5: PaymentPolicyEnforcer (Basic)

11. **Policy Enforcer Stub**  
    - [☑] Create `internal/policy/policy.go` with `PaymentPolicyEnforcer.Evaluate()` always allowing retry.
    - [☑] Write tests in `internal/policy/policy_test.go`.

---

## Chunk 6: Orchestrator (Placeholder ExecuteStep)

12. **Orchestrator Skeleton**  
    - [☑] Create `internal/orchestrator/orchestrator.go` with `Orchestrator.Execute()` looping over steps and returning success stubs.
    - [☑] Write tests in `internal/orchestrator/orchestrator_test.go`.

---

## Chunk 7: ProviderAdapter Interface & Mock Adapter

13. **Adapter Interface**  
    - [☑] Create `internal/adapter/adapter.go` defining `ProviderAdapter`.

14. **MockAdapter**  
    - [☑] Create `internal/adapter/mock/mock_adapter.go` implementing `MockAdapter`.
    - [☑] Write tests in `internal/adapter/mock/mock_adapter_test.go`.

---

## Chunk 8: Processor Layer

15. **Processor Implementation**  
    - [☑] Create `internal/processor/processor.go` with `Processor.ProcessSingleStep()`, wrapping `ProviderAdapter`.
    - [☑] Write tests in `internal/processor/processor_test.go`.

---

## Chunk 9: Router (Minimal Fallback)

16. **Router Skeleton**  
    - [☑] Create `internal/router/router.go` with simple fallback logic (primary + fallback chain).
    - [☑] Write tests in `internal/router/router_test.go`.

---

## Chunk 10: Orchestrator (Integrate Router & Policy)

17. **Orchestrator Integration**  
    - [☑] Update `internal/orchestrator/orchestrator.go` to call `Router.ExecuteStep()` and respect policy.
    - [☑] Modify `NewOrchestrator()` to accept a `Router`.
    - [☑] Write integration tests in `internal/orchestrator/orchestrator_integration_test.go`.

---

## Chunk 11: StripeAdapter (Simple Implementation)

18. **StripeAdapter**  
    - [☑] Create `internal/adapter/stripe/stripe_adapter.go` with HTTP calls and retry logic.
    - [☑] Implement helpers: `buildStripePayload()`, `generateIdempotencyKey()`.
    - [☑] Write tests in `internal/adapter/stripe/stripe_adapter_test.go` using `httptest.Server`.

---

## Chunk 12: Router (Full Features)

19. **Circuit-Breaker Service**  
    - [☑] Create `internal/router/circuitbreaker/circuitbreaker.go` with basic in-memory logic.
    - [☑] Write tests in `internal/router/circuitbreaker/circuitbreaker_test.go`.

20. **Router Enhancements**  
    - [☑] Update `internal/router/router.go` to integrate `CircuitBreakerService` and SLA budget enforcement.
    - [☑] Implement `tryFallback()` helper for marking unhealthy and skipping providers.
    - [☑] Write full-feature tests in `internal/router/router_full_test.go`, covering circuit-breaking, SLA expiration, and health checks.

---

## Chunk 13: CompositePaymentService (Full Optimizer)

21. **Simple Optimizer Logic**  
    - [☑] Update `internal/planbuilder/composite.go` to split amounts (e.g., equal split). *(Note: Logic added to `composite.go`)*
    - [☑] Write tests in `internal/planbuilder/composite_test.go` for fee-splitting logic.
    - [☑] Integrate real optimizer into `PlanBuilder.Build()` so that if metadata indicates a split, it’s applied. *(Note: Integration confirmed, no changes needed in `planbuilder.go`)*

---

## Chunk 14: PaymentPolicyEnforcer (Full DSL)

22. **DSL-Based Policy Engine**  
    - [☑] Update `internal/policy/policy.go` to use `govaluate`: compile rule expressions in `NewPolicyEnforcer(rules)`.
    - [☑] Modify `Evaluate()` to run each compiled expression against `StepExecutionContext` variables (`fraudScore`, `region`, `amount`, `merchantTier`).
    - [☑] Write tests in `internal/policy/policy_test.go` for rule evaluation (e.g., `"fraudScore > 0.8"`).

---

## Chunk 15: API Layer & Validation

23. **HTTP Server Setup**  
    - [☑] Create `cmd/server/main.go` using Gin (or similar) with endpoint `POST /process-payment`.
    - [☑] In handler: bind JSON to `external.ExternalRequest`, validate, call `BuildContexts()`, `PlanBuilder.Build()`, `Orchestrator.Execute()`, return JSON.
    - [☑] Write tests in `cmd/server/server_test.go` using `httptest` for valid and invalid payloads.

---

## Chunk 16: Observability Integration

24. **OpenTelemetry Tracing**  
    - [☑] Add OTEL initialization in `cmd/server/main.go`, wrap Gin handlers with OTEL middleware.
    - [☑] Instrument `PlanBuilder.Build()`, `Orchestrator.Execute()`, `Router.ExecuteStep()` with spans (`Tracer.Start()` / `span.End()`).

25. **Prometheus Metrics**  
    - [ ] In `internal/planbuilder/planbuilder.go`, register and increment `plan_requests_total`, record `plan_build_duration_seconds`.  
    - [ ] Write tests in `internal/planbuilder/observability_test.go` to confirm metric increments when `Build()` is called.

---

## Chunk 17: ContractMonitor & RetrospectiveReporter

26. **ContractMonitor**  
    - [ ] Create `internal/monitor/monitor.go` using `gojsonschema` to validate incoming JSON messages against a schema.  
    - [ ] Write tests in `internal/monitor/monitor_test.go` simulating valid and invalid JSON.  

27. **RetrospectiveReporter**  
    - [ ] Create `internal/reporting/retrospective.go` with `GenerateRetrospective(logs []LogEntry)`.  
    - [ ] Write tests in `internal/reporting/retrospective_test.go` for log aggregation and report contents.

---

## Chunk 18: Final Polish & Documentation

28. **README Update**  
    - [ ] Update `README.md` with:
      - Project overview referencing `spec.md`.  
      - Prerequisites (Go, Buf, golangci-lint).  
      - Build & generation instructions (`make gen`, `go build`).  
      - How to run tests (`make test`).  
      - How to start the server (`go run cmd/server/main.go`).  
      - Example `curl` usage.

29. **Package Documentation**  
    - [ ] Add top-of-file comments in each package (`internal/…/…go`) explaining its role.  
    - [ ] Ensure `spec.md` references are up to date.  

30. **CI & Coverage**  
    - [ ] Verify `make ci` (Buf codegen, lint, tests) passes.  
    - [ ] Check code coverage and aim for >90%.  

31. **Release Preparation**  
    - [ ] Tag version `v1.0.0`.  
    - [ ] Draft release notes summarizing implemented features.

---

## Legend

- ☐ = Not started / pending  
- ☑ = Completed  

Update each box as you progress. Good luck!  
```
