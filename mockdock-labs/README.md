# MockDock Target Labs

Welcome to the **MockDock Target Labs** repository! This is a collection of plug-and-play sample stacks designed as an interactive learning sandbox for container networking, dynamic SSL interception, network chaos engineering, and API mocking.

Each folder contains a fully functional test application pre-configured to work with **MockDock**.

---

## 📂 Lab Stacks Directory

### 1. [MERN To Do App](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/mern-todo/) (Stack A)
*   **Concepts Covered**: Basic MOCK/REAL orchestration, in-memory stateful REST CRUD, and static authentication mocking.
*   **Technologies**: MongoDB, Express, React, Node.js.

### 2. [Payment Portal](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/payment-portal/) (Stack B)
*   **Concepts Covered**: Dynamic SNI hijacking, local SSL Root CA trust injection, and mocking secure external SDK endpoints (Stripe/Auth0).
*   **Technologies**: Node.js, Next.js, Stripe SDK.

### 3. [Python Advice API](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/python-advice/) (Stack C)
*   **Concepts Covered**: LLM Mocking, Server-Sent Events (SSE) streaming virtualization, rate-limiting simulation, and client-side retry/timeout behavior under packet loss chaos.
*   **Technologies**: Python, FastAPI, OpenAI SDK, tenacity.

### 4. [Go Fans Microservice](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/go-fans/) (Stack D)
*   **Concepts Covered**: Strictly-typed x509 handshake verification in compiled languages (Go crypto/x509), and database infrastructure mock stubbing (PostgreSQL/Redis).
*   **Technologies**: Go (1.21+), Jackc/pgx, Redis.

---

## 📖 Lesson Worksheets

To get the most out of these target labs, follow the step-by-step guided exercises located in the [lessons/](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/lessons/) folder:

1.  **[Lesson 1: TLS Hijacking & Local CA Trust Store Injection (Node)](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/lessons/LESSON-01-TLS-HIJACKING-NODE.md)**
2.  **[Lesson 2: Chaos Engineering & Virtualized LLMs (Python)](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/lessons/LESSON-02-CHAOS-AND-LLMS-PYTHON.md)**
3.  **[Lesson 3: Infrastructure Outage & Database Mocking (Go)](file:///Users/markjordan/Documents/GitHub/mockdock/mockdock-labs/lessons/LESSON-03-INFRASTRUCTURE-FALLBACK-GO.md)**
