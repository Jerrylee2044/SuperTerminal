#!/bin/bash
# SuperTerminal Release Script
# Usage: ./scripts/release.sh [version]

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get version
if [ -z "$1" ]; then
    VERSION=$(cat VERSION)
else
    VERSION="$1"
    echo "$VERSION" > VERSION
fi

echo -e "${BLUE}=== SuperTerminal Release v${VERSION} ===${NC}"
echo ""

# Validate version format
if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: Invalid version format. Use vX.Y.Z${NC}"
    exit 1
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo -e "${YELLOW}Warning: You have uncommitted changes${NC}"
    echo "Continue anyway? (y/n)"
    read -r response
    if [[ ! "$response" =~ ^[yY]$ ]]; then
        exit 1
    fi
fi

# Run tests
echo -e "${BLUE}Running tests...${NC}"
if ! go test ./... -count=1; then
    echo -e "${RED}Tests failed! Aborting release.${NC}"
    exit 1
fi
echo -e "${GREEN}Tests passed ✓${NC}"

# Build for all platforms
echo -e "${BLUE}Building binaries...${NC}"

BUILD_DIR="build"
mkdir -p "$BUILD_DIR"

# Linux amd64
echo "  Linux amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$VERSION" -o "$BUILD_DIR/superterminal-linux-amd64" ./cmd/superterminal

# Linux arm64
echo "  Linux arm64..."
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.Version=$VERSION" -o "$BUILD_DIR/superterminal-linux-arm64" ./cmd/superterminal

# Darwin amd64
echo "  Darwin amd64..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$VERSION" -o "$BUILD_DIR/superterminal-darwin-amd64" ./cmd/superterminal

# Darwin arm64 (Apple Silicon)
echo "  Darwin arm64..."
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.Version=$VERSION" -o "$BUILD_DIR/superterminal-darwin-arm64" ./cmd/superterminal

# Windows amd64
echo "  Windows amd64..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$VERSION" -o "$BUILD_DIR/superterminal-windows-amd64.exe" ./cmd/superterminal

echo -e "${GREEN}Builds complete ✓${NC}"

# Create checksums
echo -e "${BLUE}Creating checksums...${NC}"
cd "$BUILD_DIR"
sha256sum * > checksums.sha256
cd ..
echo -e "${GREEN}Checksums created ✓${NC}"

# Update CHANGELOG
echo -e "${BLUE}Updating CHANGELOG...${NC}"
if grep -q "## \[${VERSION}\]" CHANGELOG.md; then
    echo -e "${YELLOW}Version ${VERSION} already in CHANGELOG${NC}"
else
    # Add new version entry at the top (after the header)
    sed -i "/## Version History Summary/i ## [${VERSION}] - $(date +%Y-%m-%d)\n\n### Added\n- Release v${VERSION}\n\n" CHANGELOG.md
    echo -e "${GREEN}CHANGELOG updated ✓${NC}"
fi

# Create git tag
echo -e "${BLUE}Creating git tag...${NC}"
git add VERSION CHANGELOG.md
git commit -m "Release ${VERSION}" || true
git tag -a "$VERSION" -m "Release ${VERSION}"
echo -e "${GREEN}Tag created ✓${NC}"

# Summary
echo ""
echo -e "${GREEN}=== Release Complete ===${NC}"
echo ""
echo "Version: ${VERSION}"
echo ""
echo "Binaries in ${BUILD_DIR}/:"
ls -lh "$BUILD_DIR"
echo ""
echo "Next steps:"
echo "  1. Push to GitHub: git push origin main --tags"
echo "  2. Create GitHub Release with binaries"
echo "  3. Upload binaries and checksums to release"
echo ""

# GitHub release instructions
echo -e "${YELLOW}GitHub Release Instructions:${NC}"
echo "  Title: SuperTerminal ${VERSION}"
echo "  Tag: ${VERSION}"
echo "  Files to upload:"
ls "$BUILD_DIR" | grep -v checksums
echo "  checksums.sha256"
echo ""