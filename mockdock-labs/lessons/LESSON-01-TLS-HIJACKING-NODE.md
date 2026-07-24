# Lesson 1: TLS Interception & Local Root CA Trust (Node.js)

In this lab, you will see how MockDock dynamically intercepts outbound SSL/TLS traffic, signs secure certificates on the fly, and resolves them transparently inside a Docker container without changing SDK configs or environment variables.

---

## 🧠 Core Concept: Dynamic SNI Interception & SSL Signing

When an application calls a secure external API like `https://api.stripe.com`, it undergoes a strict TLS handshake:
1.  **DNS Hijacking**: MockDock intercepts outgoing traffic for `api.stripe.com` inside the container network.
2.  **Dynamic Certificate Generation**: On-the-fly, MockDock generates an SSL certificate specifically for `api.stripe.com`.
3.  **Local Root CA signing**: The certificate is signed by MockDock's local Root Certificate Authority (CA) created on installation (`~/.mockdock/ca.crt`).
4.  **Trust Injection**: By mounting this Root CA into the container and directing the runtime (`NODE_EXTRA_CA_CERTS`), Node.js trusts the dynamically signed certificate as authentic, completing the TLS handshake cleanly.

---

## 🛠️ Step-by-Step Guided Lab

### Step 1: Control Group (Passthrough Mode)
1.  Navigate to `/mockdock-labs/payment-portal/` in your terminal.
2.  Boot the application:
    ```bash
    docker compose up --build -d
    ```
3.  Visit `http://localhost:3001` in your browser.
4.  Click the **Process Test Charge** button.
5.  **Observe**: The response payload succeeds, returning real data directly from the live Stripe sandbox. 

---

### Step 2: Enable Ghost Interception
1.  Open the MockDock Web Dashboard at `http://localhost:11800`.
2.  Go to the **Universe** tab and select the `payment-portal` workspace.
3.  Navigate to the **Workspace** screen.
4.  Find the `api.stripe.com` route service inside the endpoints list.
5.  Toggle its status from **Real** to **Ghost Mode**.
6.  Click **Restart & Rebuild** in MockDock to write the override files and restart the stack.

---

### Step 3: Verify TLS Handshake Succeeds Zero-Config
1.  Go back to your payment portal tab (`http://localhost:3001`).
2.  Click **Process Test Charge** again.
3.  **Observe**: The request succeeds instantly, but returns the MockDock fallback mock response instead of the live Stripe API!
4.  Check your Node application logs:
    ```bash
    docker logs payment-portal-web
    ```
    You will see that the Stripe Node SDK completed its connection cleanly without throwing any TLS validation errors (`UNABLE_TO_VERIFY_LEAF_SIGNATURE`).

---

## 🧪 Questions for Review
*   Why was `NODE_EXTRA_CA_CERTS` required in `docker-compose.yml`? What happens if you remove that environment variable and restart the container?
*   How did the Stripe SDK route traffic to MockDock without changing the host URL in the code?
