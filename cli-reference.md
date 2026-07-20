# MockDock CLI Command Reference

This document provides a comprehensive reference for all command-line interface (CLI) commands, flags, and configuration options available in the MockDock daemon and CLI tool.

---

## Global Commands

### `mockdock init`
Initializes a new MockDock universe workspace in the current directory.
* **Usage**: `mockdock init`
* **Details**: Creates the baseline `.mockdock` directory and registers the current folder as a compose-based workspace.

### `mockdock up`
Starts the mock compose stack inside the active workspace.
* **Usage**: `mockdock up [--build]`
* **Flags**:
  * `--build`: Force Docker to rebuild container images before starting.

### `mockdock down`
Stops the mock compose stack and cleans up running containers.
* **Usage**: `mockdock down`

### `mockdock status`
Queries the daemon to display the current state of the compose stack, active universe, and running containers.
* **Usage**: `mockdock status`

---

## Service & Stub Configuration

### `mockdock stub-config` (or `stub-wizard`)
Configures the mock engine, protocol stubs, response behaviors, and network chaos rules for a service.
* **Usage**: `mockdock stub-config --service <name> [flags]`
* **Flags**:
  * `--service`: Name of the service to configure (required).
  * `--mode`: Set service interception mode (`ghost` for mocked/redirected stubs, `real` for native container run). Default: `ghost`.
  * `--protocol`: Protocol mock engine to bind (`http`, `postgres`, `mysql`, `redis`, `mongodb`, `rabbitmq`, `tcp`, `none`). Default: `http`.
  * `--status`: Default HTTP status code to return (HTTP engine only). Default: `200`.
  * `--body`: Global JSON response body string returned as a fallback (HTTP engine only).
  * `--script`: Embedded JavaScript logic code or local path to a `.js` file to run (HTTP engine only).
  * `--crud`: Enable/disable automatic stateful in-memory CRUD collections (HTTP engine only). Default: `false`.
  * `--sqlite`: Enable SQL-to-SQLite dynamic query translation (database engines only). Default: `false`.
  * `--latency`: Latency delay in milliseconds injected at the network level.
  * `--loss`: Packet loss percentage injected at the network interface.

### `mockdock stub`
Starts a standalone, isolated listener process to mock traffic on a designated port.
* **Usage**: `mockdock stub --protocol <type> --port <port> [flags]`
* **Flags**:
  * `--protocol`: Protocol to run (`tcp`, `http`, etc.).
  * `--port`: Port to listen on.
  * `--http-status`: HTTP status code returned for requests (HTTP protocol).
  * `--response-body`: Global response body (HTTP protocol).
  * `--script`: JS script to execute for responses (HTTP protocol).
  * `--http-crud`: Enable stateful collections (HTTP protocol).
  * `--sqlite-enabled`: Enable SQLite mapping.
  * `--latency`: Latency delay in ms.
  * `--loss`: Packet loss percentage.

---

## Chaos Injection

### `mockdock chaos`
Injects real-time container-level network chaos (latency/loss) into a running service namespace.
* **Usage**: `mockdock chaos --service <name> [flags]`
* **Flags**:
  * `--service`: Service container name to target (required).
  * `--latency`: Delay to introduce in milliseconds.
  * `--loss`: Packet loss percentage to inject.
  * `--duration`: Run duration of chaos injection (e.g. `60s`, `5m`).

---

## Licensing & RAD Mode

### `mockdock license`
Manages premium MockDock license keys.
* **Subcommands**:
  * `mockdock license activate <key>`: Activates premium RAD (Rapid Application Development) features.
  * `mockdock license status`: Displays active licensing tier and status.
