#!/bin/bash
# Script to update Homebrew tap formula with new version and checksums
# Usage: ./update-homebrew-tap.sh <version> <checksums_file> <tap_repo_path>

set -e

VERSION=$1
CHECKSUMS_FILE=$2
TAP_REPO_PATH=$3
FORMULA_FILE="$TAP_REPO_PATH/Formula/mdserver.rb"

if [ -z "$VERSION" ] || [ -z "$CHECKSUMS_FILE" ] || [ -z "$TAP_REPO_PATH" ]; then
  echo "Usage: $0 <version> <checksums_file> <tap_repo_path>"
  exit 1
fi

if [ ! -f "$CHECKSUMS_FILE" ]; then
  echo "Error: Checksums file not found: $CHECKSUMS_FILE"
  exit 1
fi

if [ ! -f "$FORMULA_FILE" ]; then
  echo "Error: Formula file not found: $FORMULA_FILE"
  exit 1
fi

# Extract checksums
ARM64_MACOS=$(grep "mdserver-macos-arm64" "$CHECKSUMS_FILE" | awk '{print $1}')
AMD64_MACOS=$(grep "mdserver-macos-amd64" "$CHECKSUMS_FILE" | awk '{print $1}')
ARM64_LINUX=$(grep "mdserver-linux-arm64" "$CHECKSUMS_FILE" | awk '{print $1}')
AMD64_LINUX=$(grep "mdserver-linux-amd64" "$CHECKSUMS_FILE" | awk '{print $1}')

if [ -z "$ARM64_MACOS" ] || [ -z "$AMD64_MACOS" ] || [ -z "$ARM64_LINUX" ] || [ -z "$AMD64_LINUX" ]; then
  echo "Error: Could not extract all checksums from $CHECKSUMS_FILE"
  exit 1
fi

# Update version
sed -i.bak "s/version \"[^\"]*\"/version \"$VERSION\"/" "$FORMULA_FILE"

# Update URLs (remove 'v' prefix if present in version for URLs)
VERSION_URL=${VERSION#v}

# Update SHA256 checksums - handle both placeholder and existing checksums
sed -i.bak "s/sha256 \"[^\"]*ARM64[^\"]*\"/sha256 \"$ARM64_MACOS\"/" "$FORMULA_FILE" || \
  sed -i.bak "s/sha256 \"[^\"]*mdserver-macos-arm64[^\"]*\"/sha256 \"$ARM64_MACOS\"/" "$FORMULA_FILE" || \
  sed -i.bak "0,/sha256/s/sha256 \"[^\"]*\"/sha256 \"$ARM64_MACOS\"/" "$FORMULA_FILE"

# For macOS AMD64 - find the right line
sed -i.bak "/mdserver-macos-amd64/,/sha256/s/sha256 \"[^\"]*\"/sha256 \"$AMD64_MACOS\"/" "$FORMULA_FILE"

# For Linux ARM64
sed -i.bak "/mdserver-linux-arm64/,/sha256/s/sha256 \"[^\"]*\"/sha256 \"$ARM64_LINUX\"/" "$FORMULA_FILE"

# For Linux AMD64
sed -i.bak "/mdserver-linux-amd64/,/sha256/s/sha256 \"[^\"]*\"/sha256 \"$AMD64_LINUX\"/" "$FORMULA_FILE"

# Update download URLs
sed -i.bak "s|releases/download/v[^/]*/mdserver-macos-arm64|releases/download/$VERSION_URL/mdserver-macos-arm64|g" "$FORMULA_FILE"
sed -i.bak "s|releases/download/v[^/]*/mdserver-macos-amd64|releases/download/$VERSION_URL/mdserver-macos-amd64|g" "$FORMULA_FILE"
sed -i.bak "s|releases/download/v[^/]*/mdserver-linux-arm64|releases/download/$VERSION_URL/mdserver-linux-arm64|g" "$FORMULA_FILE"
sed -i.bak "s|releases/download/v[^/]*/mdserver-linux-amd64|releases/download/$VERSION_URL/mdserver-linux-amd64|g" "$FORMULA_FILE"

# Clean up backup files
rm -f "$FORMULA_FILE.bak"

echo "✓ Updated formula with version $VERSION"
echo "  ARM64 macOS: $ARM64_MACOS"
echo "  AMD64 macOS: $AMD64_MACOS"
echo "  ARM64 Linux: $ARM64_LINUX"
echo "  AMD64 Linux: $AMD64_LINUX"
