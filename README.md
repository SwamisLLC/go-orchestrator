# Payment Orchestration Platform

This system is a payment orchestration platform designed for resilience, modularity, and observability. It routes payments across multiple providers, handles messy inputs gracefully, enforces schema discipline, and provides built-in observability and feedback loops.

For a detailed technical blueprint and architecture, please refer to [spec2_1.md](spec2_1.md).

## Prerequisites

Before you begin, ensure you have the following installed:

- **Go**: Version 1.19 or later (check with `go version`)
- **Buf**: Version 1.0 or later (check with `buf --version`). Installation instructions can be found at [buf.build/docs/installation](https://buf.build/docs/installation).
- **golangci-lint**: Version 1.50 or later (check with `golangci-lint --version`). Installation instructions: [golangci-lint.run/usage/install/](https://golangci-lint.run/usage/install/).

## Getting Started

1.  **Clone the repository:**
    ```bash
    git clone <repository-url>
    cd payment-orchestrator
    ```

2.  **Install Go dependencies:**
    ```bash
    go mod tidy
    ```

## Build & Generation

This project uses Makefiles for common tasks.

-   **Generate code from Protobuf definitions:**
    This command uses Buf to generate Go code from the `.proto` files in the `protos/` directory.
    ```bash
    make gen
    ```

-   **Build the application:**
    This compiles the main server application.
    ```bash
    go build -o main cmd/server/main.go
    ```
    Alternatively, to build all packages:
    ```bash
    go build ./...
    ```

## Running Tests

-   **Run all tests:**
    This command will run all unit and integration tests in the project. It may also include linting as part of the CI process.
    ```bash
    make test
    ```
    To run tests for a specific package:
    ```bash
    go test ./internal/router/...
    ```

## Running the Server

-   **Start the payment orchestrator server:**
    ```bash
    go run cmd/server/main.go
    ```
    By default, the server usually starts on a port like `:8080` (this might need to be confirmed from `cmd/server/main.go` or config).

## Example Usage

You can interact with the payment processing endpoint using `curl`. Here's an example of a `POST` request to the `/process-payment` endpoint:

```bash
curl -X POST http://localhost:8080/process-payment -H "Content-Type: application/json" -d '{
  "request_id": "test-req-12345",
  "merchant_id": "merchant-007",
  "amount": 10000,
  "currency": "USD",
  "customer": {
    "name": "John Doe",
    "email": "john.doe@example.com",
    "phone": "555-1234"
  },
  "payment_method": {
    "card": {
      "card_number": "************1111",
      "expiry_month": "12",
      "expiry_year": "2025",
      "cvv": "123"
    }
  },
  "metadata": {
    "order_id": "order-abc-789",
    "product_sku": "SKU123"
  }
}'
```

---

Here’s an executive-friendly **Architecture README** — tailored to help non-engineers, product leads, and execs understand the **design philosophy**, **resilience strategy**, and **business safeguards** of your payment orchestration platform.

---

# 🧾 Architecture Overview — Payment Orchestration Platform

## 🚀 Mission

This system handles the complex world of modern digital payments — across credit cards, Apple Pay, subscriptions, fallback logic, and more — **safely, modularly, and at scale**.

It is designed to:

* **Route payments intelligently**
* **Tolerate messy or invalid inputs**
* **Support rapid growth and integrations**
* **Maintain high reliability with low latency**
* **Provide clear visibility into issues — before merchants even complain**

---

## 🧱 Core Design Principles

| Principle                      | What It Means for the Business                                                   |
| ------------------------------ | -------------------------------------------------------------------------------- |
| **Modular Architecture**       | We can evolve or replace parts (e.g., Stripe → Adyen) without system rewrites    |
| **Fault Tolerance at Edges**   | We accept messy inputs but keep the core clean and robust                        |
| **Built-in Observability**     | We know what failed, why, and where — automatically                              |
| **Domain-Driven Design (DDD)** | The architecture matches how we think about payments, processors, and policies   |
| **Schema Discipline**          | All data is version-controlled and tracked, preventing silent failures over time |

---

## 🧠 How It Works (at a Glance)

### 💬 Input

We accept payment requests from merchants — even if they're **messy**, incomplete, or slightly malformed.

### 🛡 Internal Validation

Before doing anything risky, we transform every external request into a **strict internal format**.
If something looks wrong, we:

* Keep going with best effort
* Log it
* Alert the merchant or internal team to follow up

### 🔁 Orchestration

We then:

* Choose the best available payment processor (e.g., Apple Pay first, fallback to card)
* Retry or reroute based on merchant settings
* Track every decision, latency, and outcome

### 🔍 Observability & Feedback

* We monitor every failure, slow response, and policy violation
* We generate **retrospective reports every 2 weeks**
* If merchants are unhappy but our system thinks “everything is fine,” we **flag that** so we can improve

---

## 📦 Real-World Challenges We Handle

| Real Problem                                        | How We Designed for It                                                 |
| --------------------------------------------------- | ---------------------------------------------------------------------- |
| “Merchants send bad data”                           | We accept it, warn them, and continue without system failure           |
| “Different merchants need different fallback logic” | Our routing plans are dynamic and merchant-configurable                |
| “Hard to know why a payment failed”                 | We emit structured logs, alerts, and metrics with full traceability    |
| “Changing APIs breaks everything”                   | We use versioned data contracts in a centralized schema registry       |
| “Retrospectives take too long”                      | We auto-generate a system health report every 2 weeks                  |
| “Regulators want audit logs”                        | We track every step, including retries, errors, and fallback decisions |

---

## 📊 Feedback Loops & Accountability

* **Contract Monitoring**: We define what “correct behavior” means and track violations.
* **Mismatch Detection**: If the system thinks we’re successful but users complain, we treat that as a **missed expectation**, not a win.
* **Merchant Alerts**: We notify merchants proactively when their requests contain problems — not just when things fail.

---

## ✅ What This Means for the Business

| Capability                 | Strategic Advantage                            |
| -------------------------- | ---------------------------------------------- |
| **Resilient by default**   | Accept messy merchant inputs without outages   |
| **Built for growth**       | Add processors, methods, regions without risk  |
| **Always audit-ready**     | Logs, traces, and schema versions are recorded |
| **Merchant-trust focused** | Alerts, not surprises. Feedback is systemized. |
| **Modular + future-proof** | System evolution is safe and deliberate        |

---

## 🧭 What’s Coming Next

* ML-based dynamic routing (smarter fallback)
* Real-time SLA dashboards for merchants
* Domain-specific anomaly detection
* Event replay + time-travel debugging for payment failures
