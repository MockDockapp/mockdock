# Lesson 2: Chaos Engineering & Virtualized LLMs (Python)

In this lab, you will learn how to stress test python async LLM clients under heavy rate-limiting and network degradation scenarios using MockDock's advanced chaos controls, all without spending API tokens or credit.

---

## 🧠 Core Concept: Client Resiliency under Rate-Limits & Latency

Modern AI applications heavily rely on stable connections to external LLM providers (like OpenAI or Anthropic). If a provider throttles your API key (`429 Too Many Requests`) or experiences latency spikes, your client must recover cleanly.
*   **Tenacity Retry Handles**: In `main.py`, we implement retry decorators with exponential backoff to handle transient network drops.
*   **MockDock LLM Emulation**: In Ghost Mode, MockDock intercepts `api.openai.com` requests and serves simulated chat responses.
*   **Network Chaos Injection**: MockDock uses Linux network traffic control (`tc`) to delay packets or simulate drops right inside the container interface.

---

## 🛠️ Step-by-Step Guided Lab

### Step 1: Initialize the Python Advice Stack
1.  Navigate to `/mockdock-labs/python-advice/` in your terminal.
2.  Boot the application:
    ```bash
    docker compose up --build -d
    ```
3.  Visit `http://localhost:8000` in your browser.
4.  Click the **Get AI Coding Advice** button and verify you receive a response.

---

### Step 2: Configure Virtualized LLM Mocking
1.  Open the MockDock Web Dashboard (`http://localhost:11800`).
2.  Navigate to your active `python-advice` workspace.
3.  Locate `api.openai.com` and toggle it to **Ghost Mode**.
4.  Click the **Configure** gear icon next to `api.openai.com`.
5.  In the side-sheet, click the **LLM Mocking** tab:
    *   Set **Provider** to `OpenAI`.
    *   Set **Model** to `gpt-3.5-turbo`.
    *   Toggle **Streaming (SSE)** on.
    *   Set **Time to First Token (TTFT)** to `300ms` and **Inter-Token Delay** to `50ms`.
6.  Click **Save Changes** and click **Restart & Rebuild** in the header.

---

### Step 3: Observe Streaming Mocks (SSE)
1.  Go back to your advice portal (`http://localhost:8000`).
2.  Click **Get Streaming Response (SSE)**.
3.  **Observe**: The response progressive text streams word-by-word into the output panel. MockDock successfully virtualized the SSE completion loop!

---

### Step 4: Inject Latency & Packet Drops
Now let's simulate a highly degraded, flaky network connection:
1.  On the MockDock Web Dashboard, open the **Workspace** screen.
2.  Select the `api.openai.com` service, and open its configuration gear.
3.  Click the **Chaos Control** tab:
    *   Slide **Latency** to `5000ms` (5 seconds).
    *   Set **Latency Jitter** to `500ms`.
    *   Set **Packet Loss** to `20%`.
4.  Click **Save Changes** and click **Restart & Rebuild**.
5.  Go to `http://localhost:8000` and click **Get AI Coding Advice**.
6.  **Observe**:
    *   Because latency is set to 5000ms and our code timeout limit is set to `5.0` seconds, the first request will fail.
    *   The `tenacity` retry handlers catch the failure and immediately attempt a second call.
    *   The output panel displays status statistics showing the retry loops executing in real-time.

---

## 🧪 Questions for Review
*   How did the client behave when latency exceeded the 5.0s timeout?
*   How does rate-limiting virtualization assist in testing production-ready AI pipelines?
