# Lesson 3: Database Interception & Third-Party Fallback (Go)

In this lab, you will learn how to mock stateful database dependencies (PostgreSQL/Redis) and dynamic third-party endpoints in strictly-compiled microservice environments using MockDock's query-translation and fallback engines.

---

## 🧠 Core Concept: Go x509 Certification & Infrastructure Mocking

*   **Go x509 Trust Validation**: Compiled Go code uses strict internal validation routines (`crypto/x509`) to check server certificates. To intercept HTTPS endpoints in Go, you must update the container OS CA certificate database (e.g. running `update-ca-certificates` on container start).
*   **Relational DB virtualization (SQL-to-SQLite)**: Instead of mocking postgres calls in code, MockDock intercepts PostgreSQL query packets on port 5432 and translates them in real-time to a local mock SQLite file.
*   **Redis Interception**: MockDock handles TCP proxying on port 6379, returning dummy replies or cache hits to avoid service startup crashes.

---

## 🛠️ Step-by-Step Guided Lab

### Step 1: Initialize the Go Fans Stack
1.  Navigate to `/mockdock-labs/go-fans/` in your terminal.
2.  Boot the microservice stack:
    ```bash
    docker compose up --build -d
    ```
3.  Visit `http://localhost:8080` in your browser.
4.  Click **Fetch Database & Redis Stats** and verify it reads the local PostgreSQL database tables.

---

### Step 2: Configure Database Mocking
1.  Open the MockDock Web Dashboard (`http://localhost:11800`).
2.  Select the `go-fans` workspace universe.
3.  Find the `db` (Postgres) service, and toggle its status to **Ghost Mode**.
4.  Open the configuration gear for `db`:
    *   Enable **SQLite Mapping**.
    *   In the **Init Script** box, paste the following SQL schema:
        ```sql
        CREATE TABLE pg_database (datname TEXT);
        INSERT INTO pg_database VALUES ('gofans_mock_1'), ('gofans_mock_2');
        ```
5.  Click **Save Changes** and click **Restart & Rebuild** in the header.

---

### Step 3: Verify Virtualized Database Reads
1.  Go back to the Go Fans portal (`http://localhost:8080`).
2.  Click **Fetch Database & Redis Stats**.
3.  **Observe**: The response succeeds, but the database status and DB count now reflect the values from your SQLite mock seed instead of the native PostgreSQL container!
4.  Check your Go service container logs:
    ```bash
    docker logs go-fans-web
    ```
    Notice that the Go postgres driver (`pgx`) made standard SQL query calls without needing any modification or custom base drivers.

---

### Step 4: Verify Dynamic SSL Interception
1.  On the dashboard, toggle the external route service `api.github.com` to **Ghost Mode**.
2.  Click **Restart & Rebuild**.
3.  On `http://localhost:8080`, click **Hit Secure External API**.
4.  **Observe**:
    *   The request successfully handshakes and returns a status `Success (http_code: 200)`.
    *   Under the hood, Go's `crypto/x509` package successfully validated MockDock's dynamic cert because the Alpine image's certificate store was updated with the mounted CA file.

---

## 🧪 Questions for Review
*   How did MockDock translate the SQL queries executed by the Go `pgx` driver?
*   What is the benefit of virtualizing the database at the container networking level rather than mocking database queries inside your Go code?
