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
