# MockDock Model Context Protocol (MCP) Server Reference

MockDock features a built-in **Model Context Protocol (MCP)** server, enabling AI Coding Assistants (like Cursor, Claude Desktop, and VS Code extensions) to view environment topologies, configure stubs, inject network chaos, and apply active profiles programmatically.

---

## 1. Running the MCP Server

The MCP server runs over stdio and can be started using the `mcp` subcommand:

```bash
mockdock mcp
```

---

## 2. Declared Tools Reference

The server registers the following JSON-RPC 2.0 tools with your AI environment:

### `list_services`
List all registered services in your active docker-compose environment and external SaaS definitions, showing their current status (Real/Ghost) and configurations.

* **Arguments**: None

---

### `toggle_service_mode`
Toggle a specific service between Real mode (routing direct container-to-container traffic) and Ghost mode (intercepting and routing via mock stubs).

* **Arguments**:
  * `service_name` (string, required): The service identifier.
  * `mocked` (boolean, required): Set `true` for Ghost mode, `false` for Real mode.

---

### `configure_stub`
Reconfigure a service's stub engine settings, including target mocking protocols, HTTP response parameters, or LLM mocking settings.

* **Arguments**:
  * `service_name` (string, required): Target service name.
  * `protocol` (string, required): Stub engine identifier. Supported engines:
    * `http`, `postgres`, `redis`, `mysql`, `mongodb`, `rabbitmq`, `tcp`
    * `llm_openai`, `llm_anthropic`, `llm_gemini`, `vectordb`
  * `http_status` (integer, optional): HTTP status code for HTTP mocks. Default `200`.
  * `response_body` (string, optional): HTTP response payload text.
  * `sqlite_enabled` (boolean, optional): Enable SQL-to-SQLite translation for DB stubs.
  * `llm_provider` (string, optional): Provider (e.g., `openai`, `anthropic`, `gemini`).
  * `llm_model` (string, optional): LLM model identifier (e.g., `gpt-4o`).
  * `llm_stream` (boolean, optional): Enable server-sent events (SSE) completions.

---

### `inject_chaos`
Configure network latency, packet loss percentage, jitter variance, bandwidth limiters, or cpu/memory starvation.

* **Arguments**:
  * `service_name` (string, required): Target service name.
  * `latency_ms` (integer, optional): Latency delay in milliseconds.
  * `packet_loss_pct` (integer, optional): Packet loss rate in percentage (0-100).
  * `latency_jitter` (integer, optional): Latency jitter variation in milliseconds.
  * `bandwidth` (integer, optional): Bandwidth rate limit (kbps).
  * `cpu_spike_pct` (integer, optional): Simulated CPU spike cycles workload (0-100).
  * `mem_spike_mb` (integer, optional): Simulated memory allocation consumption (MB).

---

### `rebuild_universe`
Regenerate environment overrides and rebuild the active Docker Compose universe containers to apply new stubs/chaos changes.

* **Arguments**: None

---

## 3. Configuring IDE Integrations

### Claude Desktop Configuration
Add the following configuration block to your Claude Desktop config file (located at `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mclientServers": {
    "mockdock-mcp": {
      "command": "/usr/local/bin/mockdock",
      "args": ["mcp"]
    }
  }
}
```

### Cursor configuration
1. Navigate to **Cursor Settings** > **Features** > **MCP**.
2. Click **+ Add New MCP Server**.
3. Configure the following parameters:
   * **Name**: `MockDock`
   * **Type**: `stdio`
   * **Command**: `/usr/local/bin/mockdock mcp`
