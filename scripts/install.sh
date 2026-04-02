#!/bin/bash
# SuperTerminal Installation Script
# Usage: curl -sL https://raw.githubusercontent.com/yourname/SuperTerminal/main/scripts/install.sh | bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

REPO_URL="https://github.com/yourname/SuperTerminal"
INSTALL_DIR="/usr/local/bin"
BIN_NAME="superterminal"

echo -e "${BLUE}=== SuperTerminal Installer ===${NC}"
echo ""

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) echo -e "${RED}Unsupported OS: $OS${NC}"; exit 1 ;;
esac

echo -e "${GREEN}Detected: ${OS}/${ARCH}${NC}"

# Get latest version
echo -e "${BLUE}Fetching latest version...${NC}"
LATEST_VERSION=$(curl -sI "https://github.com/yourname/SuperTerminal/releases/latest" | grep -i "location:" | sed -E 's/.*\/(v[0-9]+\.[0-9]+\.[0-9]+).*/\1/')
if [ -z "$LATEST_VERSION" ]; then
    LATEST_VERSION="v0.4.0"
fi
echo -e "${GREEN}Latest version: ${LATEST_VERSION}${NC}"

# Download binary
BINARY_URL="${REPO_URL}/releases/download/${LATEST_VERSION}/superterminal-${OS}-${ARCH}"
echo -e "${BLUE}Downloading: ${BINARY_URL}${NC}"

TEMP_FILE="/tmp/superterminal-${OS}-${ARCH}"

if ! curl -sL "$BINARY_URL" -o "$TEMP_FILE"; then
    echo -e "${RED}Download failed!${NC}"
    exit 1
fi

# Verify download
if [ ! -s "$TEMP_FILE" ]; then
    echo -e "${RED}Downloaded file is empty!${NC}"
    exit 1
fi

echo -e "${GREEN}Download complete ✓${NC}"

# Make executable
chmod +x "$TEMP_FILE"

# Install
echo -e "${BLUE}Installing to ${INSTALL_DIR}...${NC}"

if [ -w "$INSTALL_DIR" ]; then
    mv "$TEMP_FILE" "${INSTALL_DIR}/${BIN_NAME}"
else
    echo -e "${YELLOW}Need sudo to install to ${INSTALL_DIR}${NC}"
    sudo mv "$TEMP_FILE" "${INSTALL_DIR}/${BIN_NAME}"
fi

echo -e "${GREEN}Installation complete ✓${NC}"

# Verify installation
if ! command -v superterminal &> /dev/null; then
    echo -e "${YELLOW}Note: ${INSTALL_DIR} may not be in your PATH${NC}"
    echo "Add it to PATH or use: ${INSTALL_DIR}/superterminal"
fi

# Check for API key
echo ""
echo -e "${BLUE}=== Setup ===${NC}"
if [ -z "$ANTHROPIC_API_KEY" ]; then
    echo -e "${YELLOW}API key not set.${NC}"
    echo "Set your Anthropic API key:"
    echo "  export ANTHROPIC_API_KEY=sk-ant-xxxxx"
    echo ""
    echo "Or add to your shell config (~/.bashrc or ~/.zshrc):"
    echo "  echo 'export ANTHROPIC_API_KEY=sk-ant-xxxxx' >> ~/.bashrc"
fi

# Show usage
echo ""
echo -e "${GREEN}=== Ready to Use ===${NC}"
echo ""
echo "Quick start:"
echo "  superterminal           # Terminal UI"
echo "  superterminal --web     # Terminal + Web UI"
echo "  superterminal --help    # Show all options"
echo ""

# Version info
INSTALLED_VERSION=$(superterminal --version 2>/dev/null || echo "${LATEST_VERSION}")
echo -e "${GREEN}Installed: SuperTerminal ${INSTALLED_VERSION}${NC}"
echo ""

# Create config directory
CONFIG_DIR="${HOME}/.superterminal"
mkdir -p "$CONFIG_DIR"

if [ ! -f "${CONFIG_DIR}/config.json" ]; then
    echo -e "${BLUE}Creating default config...${NC}"
    cat > "${CONFIG_DIR}/config.json" << 'EOF'
{
  "api_key": "",
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 4096,
  "permission_mode": "ask",
  "show_cost": true,
  "show_tokens": true,
  "auto_save": true,
  "web_port": 8080
}
EOF
    echo -e "${GREEN}Config created at ${CONFIG_DIR}/config.json${NC}"
fi

echo ""
echo -e "${GREEN}=== Installation Complete ===${NC}"
echo ""
echo "Happy coding with SuperTerminal! 🚀"