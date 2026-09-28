#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
VSIX_PATH="$SCRIPT_DIR/vscode-ruby-lsp-go.vsix"
GO_COMMAND="${GO_COMMAND:-go}"

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
  exit 1
fi

rm -rf "$BIN_DIR"
mkdir -p "$BIN_DIR"
echo "Building platform binaries..."
for target in "${TARGETS[@]}"; do
  IFS=":" read -r go_target output_target binary_name <<< "$target"
  IFS="/" read -r target_os target_arch <<< "$go_target"
  target_dir="$BIN_DIR/$output_target"
  mkdir -p "$target_dir"
  echo "  $output_target"
  GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED=0 \
    "$GO_COMMAND" build -trimpath -ldflags "-s -w" \
    -o "$target_dir/$binary_name" "$PROJECT_DIR"
  if [[ "$target_os" != "windows" ]]; then
    chmod +x "$target_dir/$binary_name"
  fi
done

cd "$SCRIPT_DIR"
echo "Installing Node dependencies..."
npm ci --no-audit --no-fund --prefer-offline

if [[ "${SKIP_NPM_AUDIT:-0}" != "1" ]]; then
  echo "Auditing production dependencies..."
  npm audit --omit=dev --audit-level=moderate
else
  echo "Skipping npm audit because SKIP_NPM_AUDIT=1."
fi

echo "Compiling TypeScript..."
npm run compile

echo "Packaging extension..."
rm -f "$VSIX_PATH"
npx --yes @vscode/vsce package \
  --ignoreFile .vscodeignore \
  --out "$VSIX_PATH"

python3 - "$VSIX_PATH" "$SCRIPT_DIR/package.json" <<'PY'
import json
import sys
import zipfile

vsix_path, package_path = sys.argv[1:]
with open(package_path, encoding="utf-8") as handle:
    version = json.load(handle)["version"]

expected = {
    "extension/bin/darwin-x64/ruby-lsp-go",
    "extension/bin/darwin-arm64/ruby-lsp-go",
    "extension/bin/linux-x64/ruby-lsp-go",
    "extension/bin/linux-arm64/ruby-lsp-go",
    "extension/bin/win32-x64/ruby-lsp-go.exe",
    "extension/bin/win32-arm64/ruby-lsp-go.exe",
}
with zipfile.ZipFile(vsix_path) as archive:
    names = set(archive.namelist())
    missing = sorted(expected - names)
    if missing:
        raise SystemExit("VSIX is missing platform binaries: " + ", ".join(missing))
    manifest = json.loads(archive.read("extension/package.json"))
    if manifest.get("version") != version:
        raise SystemExit("VSIX package.json version does not match the source manifest")
PY

echo "Extension packaged successfully: $VSIX_PATH"
