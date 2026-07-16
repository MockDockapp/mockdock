# MockDock app

> **Local Service Virtualization & Network Sandboxing, Perfected.**

MockDock is a desktop sandbox engine that allows developers to mock third-party APIs, simulate network failures, share state across stubs, and run security audits on docker-compose stacks—completely offline.

---

## 🚀 Key Features

*   **Dynamic SNI Hijacking**: Intercept HTTP/HTTPS calls to external domains (like Stripe, Auth0, or Twilio) directly at the container edge with zero configuration changes.
*   **Dynamic SSL Signing**: Automatically signs SSL certificates on-the-fly using a trusted local Root CA.
*   **Chaos Engineering**: Ingest packet loss, latency, and sudden disconnections into your containers to test application resilience.
*   **Goja JS Engine Scripting**: Write lightweight JS scripts to power stateful stubs, complete with namespaced state cache persistence and fetch clients.
*   **Security Auditing**: Scan your `docker-compose.yml` configurations for vulnerable root privileges, open port leaks, and insecure credentials.

---

## 🛠️ Installation

Before installing, check the [Environment Readiness Checklist](https://github.com/MockDockapp/mockdock/blob/main/readiness-checklist.md) to ensure your Docker Desktop environment is configured correctly.

To download and install MockDock, run the following in your terminal:

```bash
curl -sSL https://mockdockapp.github.io/install.sh | bash
```

This installs the `mockdock` CLI tool and boots the background daemon. 

---

## 🚦 Quick Start

1.  **Launch Dashboard**: Open your browser to [http://localhost:11800](http://localhost:11800).
2.  **Add a Universe**: Go to the **Profiles** tab, input a path to your project's `docker-compose.yml` (locally or via URL), and activate the universe.
3.  **Configure Ghost Mocks**: Head to **Workspace**, toggle any service into **Ghost Mode (Stub)**, and click **Configure** to define responses or JS behavior.
4.  **Rebuild**: Click **Restart & Rebuild** to swap real containers with lightweight MockDock stubs dynamically.
5.  **Save Setup**: Go to the **Profiles** tab and save the mock configuration state to reuse later.

---

## 📖 Documentation & Support

*   Refer to the [Readiness Checklist](https://github.com/MockDockapp/mockdock/blob/main/readiness-checklist.md) for environment configuration.
*   For questions or issues, visit the [MockDock app Organization](https://github.com/MockDockapp).
