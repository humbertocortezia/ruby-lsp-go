#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
GO_COMMAND="${GO_COMMAND:-go}"

# A VS Code extension installed on another machine cannot use the binary that
# was built on the developer's machine. Package one binary for every platform
# and architecture supported by the extension instead.
TARGETS=(
  "darwin/amd64:darwin-x64:ruby-lsp-go"
  "darwin/arm64:darwin-arm64:ruby-lsp-go"
  "linux/amd64:linux-x64:ruby-lsp-go"
  "linux/arm64:linux-arm64:ruby-lsp-go"
  "windows/amd64:win32-x64:ruby-lsp-go.exe"
  "windows/arm64:win32-arm64:ruby-lsp-go.exe"
)

echo "Building VS Code extension for Ruby LSP Go..."

if ! command -v "$GO_COMMAND" >/dev/null 2>&1; then
  echo "Error: Go 1.21+ is required to bundle the Ruby LSP Go server." >&2
  echo "Install it with 'brew install go' or from https://go.dev/doc/install, then run ./build.sh again." >&2
  exit 1
fi

mkdir -p "$BIN_DIR"

echo "Building platform binaries..."
for target in "${TARGETS[@]}"; do
  IFS=":" read -r go_target output_target binary_name <<< "$target"
  target_dir="$BIN_DIR/$output_target"
  mkdir -p "$target_dir"

  IFS="/" read -r target_os target_arch <<< "$go_target"
  echo "  $output_target"
  GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED=0 \
    "$GO_COMMAND" build -trimpath -ldflags "-s -w" \
    -o "$target_dir/$binary_name" "$PROJECT_DIR"

  if [[ "$target_os" != "windows" ]]; then
    chmod +x "$target_dir/$binary_name"
  fi
done

# Install dependencies
cd "$SCRIPT_DIR"
npm install

# Compile TypeScript
npm run compile

# Package extension
npx @vscode/vsce package --ignoreFile .vscodeignore

echo "Extension packaged with platform binaries as vscode-ruby-lsp-go-*.vsix"
