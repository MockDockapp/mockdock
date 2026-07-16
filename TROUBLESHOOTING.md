# MockDock Installation Troubleshooting Guide

This guide addresses the most common installation issues encountered by beta testers running the MockDock app on macOS and Linux.

---

## 1. Error: "Docker is not installed" (When it actually is)

### Symptom:
The installer script exits immediately with:
`❌ Error: Docker is not installed.`

### Cause:
This happens if you run the installer script using `sudo bash install.sh` or `sudo curl...`. 
On macOS, running a script with `sudo` strips out user-level environment paths. Because Docker Desktop installs command symlinks in user directories, the root user cannot locate the `docker` command.

### Solution:
Run the installer **without `sudo`**. The script will prompt you for your administrator password only when copying the binary to `/usr/local/bin`:
```bash
curl -sSL https://mockdockapp.github.io/install.sh | bash
```

---

## 2. Error: "ports are not available: connecting to com.docker.vmnetd.sock"

### Symptom:
Docker returns an error similar to:
`error response from daemon: ports are not available: exposing port TCP 127.0.0.1:80... connect: no such file or directory`

### Cause:
MockDock binds to host ports **80** and **443** on localhost to intercept local HTTP/HTTPS microservice calls. On macOS, binding to privileged ports (< 1024) requires Docker Desktop to use a privileged system helper daemon (`com.docker.vmnetd`). If this helper is not allowed, the container fails to start.

### Solution:
1. Open **Docker Desktop**.
2. Click the **Gear Icon** (Settings) -> **General** (or **Advanced** depending on version).
3. Check the box to **"Allow privileged port binding (ports < 1024)"** or **"Use virtualisation framework"**.
4. Apply and restart. Enter your Mac password when prompted to authorize the helper tool.
5. Rerun the installer.

---

## 3. Error: "operation not permitted" (Mount source path errors)

### Symptom:
The installer container fails with:
`Error response from daemon: error while creating mount source path 'host_mnt...' mkdir 'host_mnt...' operation not permitted`

### Cause:
MockDock mounts your `~/Documents` directory so that it can dynamically locate and parse your `docker-compose.yml` worktrees. Newer versions of macOS block applications (including Docker Desktop) from mounting folders like `Documents`, `Desktop`, or `Downloads` without explicit permission.

### Solution:
1. Open **System Settings** on your Mac.
2. Navigate to **Privacy & Security** -> **Files and Folders** (or **Full Disk Access**).
3. Toggle the switch for **Docker** (or Docker Desktop) to **ON** (enable Documents access).
4. **Restart Docker Desktop** to apply permissions.
5. **Delete any stale container**: `docker rm -f mockdock`
6. Rerun the installer.

---

## 4. Error: "pull access denied / unauthorized"

### Symptom:
Docker returns:
`Error response from daemon: pull access denied... repository does not exist or may require 'docker login'`

### Cause:
The package image on the GitHub Container Registry is private by default, requiring authenticated credentials to fetch it.

### Solution:
Ensure that the repository organization administrator has changed the package visibility settings on GitHub (`mockdock` package settings) from **Private** to **Public**. Once changed, no `docker login` is needed to pull the image.
