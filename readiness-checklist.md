# MockDock Environment Readiness Checklist

Before installing or launching the MockDock app, run through this quick checklist to ensure your local machine and Docker daemon are fully prepared.

---

## 💻 1. Host Machine Prerequisites

*   [ ] **Command Path Verification**
    *   Verify that `docker` is available in your user session's `PATH` without `sudo` privileges.
    *   *Check*: Run `docker --version` in your terminal. If it says "command not found," re-enable command-line links in your Docker Desktop settings.
*   [ ] **No Sudo for Installation**
    *   Do **NOT** run the installation script using `sudo bash` or `sudo curl`. The installer runs in user space and will prompt for credentials only when registering files. Running with `sudo` will strip your user `PATH` and cause Docker checks to fail.

---

## 🐳 2. Docker Daemon Settings

*   [ ] **Active Status**
    *   Ensure Docker Desktop is active and fully running.
    *   *Check*: Run `docker info` to verify that the daemon is accepting socket connections.
*   [ ] **Privileged Port Binding (macOS)**
    *   Since MockDock intercepts HTTP/HTTPS requests on ports **80** and **443**, you must allow Docker to bind to privileged ports (< 1024).
    *   *Enable*: Open **Docker Desktop Settings (Gear Icon)** -> **General** (or **Advanced**) -> Check **"Allow privileged port binding (ports < 1024)"** or **"Use virtualisation framework"**. Enter your Mac password when prompted.
*   [ ] **Default Docker Socket Symlink (macOS)**
    *   MockDock (running inside its container) needs access to the host's Docker socket to communicate with Docker Desktop and spin up Ghost Mode stubs.
    *   *Enable*: Open **Docker Desktop Settings (Gear Icon)** -> **Advanced** -> Check **"Allow the default Docker socket to be used"** (this creates the `/var/run/docker.sock` symlink and may prompt for your Mac password).
*   [ ] **Clean Container Namespace**
    *   Ensure there are no existing or stale container instances named `mockdock`.
    *   *Check*: Run `docker rm -f mockdock` in your terminal to clear any remnants before running the installer.

---

## 🔒 3. OS-level Permissions (macOS Sonoma / Sequoia / Ventura)

*   [ ] **Directory Mount Access**
    *   MockDock mounts your `~/Documents` directory to read and hot-reload your compose profiles. You must grant Docker permission to access this folder.
    *   *Enable*: Open **System Settings** -> **Privacy & Security** -> **Files and Folders** (or **Full Disk Access**) -> Toggle the switch for **Docker** to **ON**.
    *   *Restart*: Restart Docker Desktop after making this change.

---

## 📂 4. MockDock Directory Footprint

MockDock creates and works with specific directories and files during its operation:
*   **Global Configuration Cache (`~/.mockdock/`)**:
    *   Located in your user home directory.
    *   Contains the `global-config.json` registry of project universes, your CA certificates (`ca.crt` / `ca.key`), and local stubs catalog cache.
*   **Local Project Workspace Config (`[project-root]/mockdock/`)**:
    *   Created in each project folder when running `mockdock init`.
    *   Stores cached compose files (`source-compose.yaml`, `mocked-compose.yaml`), dynamic sources registry (`sources.json`), active stubs state (`active_stubs.json`), and custom Javascript mock files.
*   **CLI Binary Command (`/usr/local/bin/mockdock`)**:
    *   Registered globally in your OS command path to run MockDock CLI helper commands.
*   **Docker Daemon Container**:
    *   A container named `mockdock` running locally in the background.

---

## 🧹 5. How to Fully Remove MockDock

To completely uninstall MockDock and purge all local configuration footprints from your machine, run these commands in your terminal:

```bash
# 1. Stop and remove the background daemon container
docker rm -f mockdock

# 2. Delete the global configuration and certificate cache directory
rm -rf ~/.mockdock

# 3. Delete the globally registered CLI helper binary
sudo rm -f /usr/local/bin/mockdock

# 4. (Optional) Remove local MockDock configuration in specific project folders
rm -rf /path/to/your/project/mockdock
```

---

## 📦 6. Running Stacks Without Local Source Code (Ghost Mode)

When initializing MockDock in a folder that contains a `docker-compose.yml` file but **lacks** the actual application source code directories (for example, when testing docker-compose recipes from repositories like *awesome-compose*):

1. **Understand Build Failures**:
   * If a service defines a `build:` property (such as `build: ./frontend` or `build: .`), Docker Compose expects to find the `Dockerfile` and the build folder locally on your disk.
   * If those directories are missing, starting the stack will fail with a command error (such as **exit status 17** or `unable to prepare context`).

2. **How to Bypass Using Ghost Mode**:
   * Open the MockDock Web Dashboard at `http://localhost:11800`.
   * Navigate to your active Universe workspace.
   * In the services list table, locate any service that uses a local build path (indicated by a `build:` key in the original compose file).
   * Toggle that service's mode from **Real** to **Ghost** (mocked).
   * **What MockDock Does**: When a service is in Ghost mode, MockDock's compiler automatically strips the `build` property from the overrides and swaps the container image with the pre-built `ghcr.io/mockdockapp/mockdock` mock mockup image.
   * Toggle the stack **ON** or click **Rebuild Stack**. Docker Compose will now start the container instantly without attempting to build any local code!

