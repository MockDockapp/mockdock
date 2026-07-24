#!/bin/bash
set -e

# MockDock Installer Script
# Detects OS, Architecture, downloads the CLI, checks Docker, and bootstraps the Daemon.

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0;37m' # No Color

echo -e "${BLUE}===============================================${NC}"
echo -e "${BLUE}        🚀 Installing MockDock CLI...          ${NC}"
echo -e "${BLUE}===============================================${NC}"

# 1. OS and Architecture Detection
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  darwin)
    PLATFORM="darwin"
    ;;
  linux)
    PLATFORM="linux"
    ;;
  *)
    echo -e "${RED}❌ Error: Unsupported OS: $OS. MockDock currently supports macOS and Linux.${NC}"
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64)
    BINARY_ARCH="amd64"
    ;;
  arm64|aarch64)
    BINARY_ARCH="arm64"
    ;;
  *)
    echo -e "${RED}❌ Error: Unsupported CPU architecture: $ARCH.${NC}"
    exit 1
    ;;
esac

# 2. Check Docker dependency
echo -e "🔍 Checking Docker installation..."
if ! command -v docker &> /dev/null; then
    echo -e "${RED}❌ Error: Docker is not installed. MockDock requires Docker Desktop or Docker Engine to run.${NC}"
    echo -e "${YELLOW}Please install Docker from https://www.docker.com and try again.${NC}"
    exit 1
fi

if ! docker info &> /dev/null; then
    echo -e "${RED}❌ Error: Docker daemon is not running. Please start Docker and try again.${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Docker is active and running.${NC}"

# 3. Setup CLI binary
INSTALL_DIR="/usr/local/bin"
TARGET_PATH="$INSTALL_DIR/mockdock"

echo -e "📦 Preparing MockDock CLI binary..."
# Check if we are running in the MockDock dev repository
if [ -f "./mockdock" ]; then
    echo -e "ℹ️  Using locally compiled dev binary."
    sudo cp ./mockdock "$TARGET_PATH"
else
    DOWNLOAD_URL="https://github.com/MockDockapp/mockdock/releases/download/v1.0.0-beta/mockdock-${PLATFORM}-${BINARY_ARCH}"
    echo -e "📥 Downloading release from $DOWNLOAD_URL..."
    sudo curl -sSL -o "$TARGET_PATH" "$DOWNLOAD_URL"
fi

sudo chmod +x "$TARGET_PATH"
echo -e "${GREEN}✅ Installed MockDock CLI to $TARGET_PATH${NC}"

# 4. Pull Daemon Container
echo -e "🐳 Pulling MockDock Daemon image..."
# Check if we have source files here (development environment)
if [ -f "Dockerfile" ]; then
    if docker image inspect ghcr.io/mockdockapp/mockdock:latest &> /dev/null; then
        echo -e "${GREEN}✅ Local ghcr.io/mockdockapp/mockdock:latest container image found.${NC}"
    else
        echo -e "🔨 Building ghcr.io/mockdockapp/mockdock:latest local image..."
        docker build -t ghcr.io/mockdockapp/mockdock:latest .
    fi
else
    echo -e "📥 Pulling latest ghcr.io/mockdockapp/mockdock:latest image from GitHub Packages..."
    docker pull ghcr.io/mockdockapp/mockdock:latest
fi

# 5. Boot / Bootstrap Daemon
echo -e "⚙️  Bootstrapping MockDock daemon service..."

# Stop and remove existing mockdock container to ensure clean state
if docker ps -a | grep mockdock &> /dev/null; then
    echo -e "🔄 Resetting existing MockDock daemon container..."
    docker rm -f mockdock &> /dev/null
fi

# Determine the real user home directory
if [ -n "$SUDO_USER" ]; then
    REAL_HOME=$(eval echo ~$SUDO_USER)
else
    REAL_HOME="$HOME"
fi

mkdir -p "$REAL_HOME/.mockdock"
if [ -n "$SUDO_USER" ]; then
    chown -R "$SUDO_USER" "$REAL_HOME/.mockdock"
fi

# Detect Docker socket path
DOCKER_SOCK="/var/run/docker.sock"
if [ -S "$REAL_HOME/.orbstack/run/docker.sock" ]; then
    DOCKER_SOCK="$REAL_HOME/.orbstack/run/docker.sock"
elif [ -S "$REAL_HOME/.colima/default/docker.sock" ]; then
    DOCKER_SOCK="$REAL_HOME/.colima/default/docker.sock"
fi
echo -e "🐳 Mapping Docker socket: $DOCKER_SOCK"

docker run -d --name mockdock \
  -e MOCKDOCK_BIND_ADDR=0.0.0.0:11800 \
  -v "$DOCKER_SOCK":/var/run/docker.sock \
  -v "$REAL_HOME/.mockdock":/root/.mockdock \
  -v "$REAL_HOME":"$REAL_HOME" \
  -p 127.0.0.1:11800:11800 \
  -p 127.0.0.1:80:80 \
  -p 127.0.0.1:443:443 \
  ghcr.io/mockdockapp/mockdock:latest

echo -e "${GREEN}🚀 MockDock daemon started successfully!${NC}"

# Automatically download Target Labs
if command -v mockdock &> /dev/null; then
    echo -e "📥 Downloading MockDock Target Labs..."
    mockdock test-labs-install
fi

echo -e "\n${GREEN}===============================================${NC}"
echo -e "${GREEN}🎉 MockDock is successfully installed!        ${NC}"
echo -e "   To get started:"
echo -e "   1. Navigate to a target lab inside './mockdock-labs/'"
echo -e "   2. Run: mockdock init"
echo -e "   3. Run: docker compose up -d"
echo -e "   4. Open http://localhost:11800 to load the dashboard."
echo -e "${GREEN}===============================================${NC}"
