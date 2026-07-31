#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGET_PLATFORM="${VSCODE_TARGET:-linux-x64}"

if [[ "${TARGET_PLATFORM}" != "linux-x64" ]]; then
  echo "Error: this build currently supports only the linux-x64 VS Code target." >&2
  exit 1
fi

cd "${SCRIPT_DIR}"

echo "Building VS Code extension for Ruby LSP Go..."

# Install exactly the lockfile without audit/funding noise. Vulnerability
# review belongs in CI, not in the packaging step.
npm ci --no-audit --no-fund --loglevel=error

# Embed the server when Go is available. A prebuilt binary is accepted for
# environments that only package the extension (for example Open VSX CI).
if command -v go >/dev/null 2>&1; then
  mkdir -p "${SCRIPT_DIR}/bin"
  (
    cd "${PROJECT_ROOT}"
    GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "${SCRIPT_DIR}/bin/ruby-lsp-go" .
  )
elif [[ ! -x "${SCRIPT_DIR}/bin/ruby-lsp-go" ]]; then
  echo "Error: Go is not installed and bin/ruby-lsp-go does not exist." >&2
  exit 1
fi

# Type-check and bundle the extension into one runtime JavaScript file.
npm run compile

# Package using the declared, current VSCE implementation without a prompt.
npx --no-install @vscode/vsce package --no-dependencies --target "${TARGET_PLATFORM}"

PACKAGE_VERSION="$(node -p "require('./package.json').version")"
echo "Extension packaged as vscode-ruby-lsp-go-${TARGET_PLATFORM}-${PACKAGE_VERSION}.vsix"
