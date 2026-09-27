#!/bin/bash

set -e

echo "Building VS Code extension for Ruby LSP Go..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BINARY_PATH="$SCRIPT_DIR/bin/ruby-lsp-go"
VSIX_PATH="$SCRIPT_DIR/vscode-ruby-lsp-go.vsix"

# Step 1: Build the Go binary
build_go_binary() {
	cd "$PROJECT_ROOT"
	if command -v go &>/dev/null; then
		GO_BIN="$(command -v go)"
	else
		GO_BIN=""
		for candidate in \
			/usr/local/go/bin/go \
			/usr/lib/go/bin/go \
			"$HOME/go/bin/go" \
			"$HOME/.go/bin/go" \
			/snap/bin/go; do
			if [ -x "$candidate" ]; then
				GO_BIN="$candidate"
				break
			fi
		done
	fi

	if [ -z "$GO_BIN" ]; then
		echo ""
		echo "ERROR: Go not found in PATH."
		echo ""
		echo "Install on Ubuntu/WSL:"
		echo "  sudo apt update && sudo apt install -y golang-go"
		echo "  # ou versão mais recente:"
		echo "  wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz"
		echo "  sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz"
		echo "  export PATH=\$PATH:/usr/local/go/bin"
		echo ""
		if [ -f "$BINARY_PATH" ]; then
			BINARY_DATE="$(stat -c '%y' "$BINARY_PATH" 2>/dev/null || stat -f '%Sm' "$BINARY_PATH" 2>/dev/null || echo 'unknown')"
			echo "WARNING: Using EXISTING binary from $BINARY_DATE"
			echo "         It may be outdated. Install Go and re-run ./build.sh"
			echo ""
		else
			echo "No binary at $BINARY_PATH — cannot continue."
			exit 1
		fi
		return
	fi

	echo "Using Go: $GO_BIN ($("$GO_BIN" version))"
	# CGO_ENABLED=0: funciona sem gcc; usa parser Go interno (sem tree-sitter nativo)
	CGO_ENABLED=0 "$GO_BIN" build -ldflags="-s -w" -o "$BINARY_PATH" .
	chmod +x "$BINARY_PATH"
	echo "Go binary built successfully ($(du -h "$BINARY_PATH" | cut -f1))"
}

# Step 2: Install Node dependencies (idempotent and offline-friendly)
install_node_deps() {
	cd "$SCRIPT_DIR"
	if [ -d node_modules ] && [ -f package-lock.json ]; then
		echo "Node modules already present, skipping npm install."
		return
	fi
	echo "Installing Node dependencies (this can take a minute)..."
	# --no-audit avoids hitting the registry twice.
	# --no-fund avoids funding messages.
	# --no-progress disables the progress bar that can confuse terminals.
	# We use --prefer-offline if available to avoid hanging on slow networks.
	npm install --no-audit --no-fund --no-progress --loglevel=error
}

# Step 3: Validate production dependency security.
# Runs `npm audit` and fails the build on any moderate+ vulnerability.
audit_deps() {
	cd "$SCRIPT_DIR"
	if ! command -v python3 >/dev/null 2>&1; then
		echo "Skipping audit (python3 not available for JSON parsing)."
		return
	fi
	echo "Auditing production dependencies..."
	local audit_json
	audit_json="$(npm audit --omit=dev --json 2>/dev/null || echo '{}')"
	local vuln_count
	vuln_count="$(echo "$audit_json" | python3 -c "import sys,json
try:
    d=json.load(sys.stdin)
    print(sum(len(v.get('via', [])) for v in d.get('vulnerabilities', {}).values()))
except Exception:
    print(0)" 2>/dev/null || echo 0)"
	if [ "${vuln_count:-0}" -gt 0 ]; then
		echo ""
		echo "ERROR: $vuln_count production vulnerabilities found. Aborting build."
		echo "$audit_json" | python3 -c "import sys,json
try:
    d=json.load(sys.stdin)
    for name,v in d.get('vulnerabilities', {}).items():
        print('-', name, v.get('severity','?'))
except Exception:
    pass" 2>/dev/null || true
		exit 1
	fi
	echo "No production vulnerabilities."
}

# Step 4: Compile TypeScript
compile_typescript() {
	cd "$SCRIPT_DIR"
	echo "Compiling TypeScript..."
	npm run compile
}

# Step 5: Package extension (non-interactive)
package_extension() {
	cd "$SCRIPT_DIR"
	echo "Packaging extension..."
	rm -f "$VSIX_PATH"
	npx --yes @vscode/vsce package \
		--out "$VSIX_PATH" \
		--allow-missing-repository
}

# Step 6: Validate that the binary actually made it into the .vsix
validate_vsix() {
	if ! command -v python3 >/dev/null 2>&1; then
		return
	fi
	if ! python3 -c "import zipfile; z=zipfile.ZipFile('$VSIX_PATH'); assert 'extension/bin/ruby-lsp-go' in z.namelist()" 2>/dev/null; then
		echo ""
		echo "ERROR: $VSIX_PATH was built but the Go binary is missing from it."
		exit 1
	fi
}

# -- main -------------------------------------------------------------------

build_go_binary
install_node_deps
audit_deps
compile_typescript
package_extension
validate_vsix

echo ""
echo "================================================"
echo "Extension packaged successfully!"
echo "Binary: $BINARY_PATH"
echo "VSIX:   $VSIX_PATH"
echo ""
echo "Install:"
echo "  code --install-extension $VSIX_PATH"
echo ""
echo "Test in VS Code:"
echo "  1. Open a Ruby project"
echo "  2. Output panel > 'Ruby LSP Go' — check server started"
echo "  3. Try: go-to-definition, folding, completion"
echo "================================================"
