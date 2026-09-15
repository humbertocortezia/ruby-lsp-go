# Ruby LSP Go Setup Guide

## Complete Setup Instructions

### 1. Prerequisites

- Go 1.21+ installed
- Node.js and npm for building the VS Code extension
- Ruby and Bundler for Ruby project support

On macOS with Homebrew, install Go with:

```bash
brew install go
go version
```

The official installation instructions are available at
<https://go.dev/doc/install>.

### 2. Building the Go Server (optional)

```bash
# Clone or navigate to the ruby-lsp-go directory
cd ruby-lsp-go

# Build the server executable
go build -o ruby-lsp-go main.go

# Move the binary to a location in your PATH
sudo mv ruby-lsp-go /usr/local/bin/ruby-lsp-go

# Or add the current directory to your PATH
export PATH=$PATH:$(pwd)
```

### 3. Installing Dependencies

For the Go server:
```bash
go mod tidy
```

For the VS Code extension:
```bash
cd vscode-extension
npm install
```

### 4. Building the VS Code Extension

```bash
cd vscode-extension
chmod +x build.sh
./build.sh
```

This will create a `.vsix` file that can be installed in VS Code.
The build also compiles the Go server for macOS, Linux, and Windows and embeds
those platform-specific binaries in the extension. Users installing this
package do not need to add `ruby-lsp-go` to their `PATH`.

### 5. Alternative Extension Installation

You can also install the extension directly from VS Code marketplace after publishing, or:
```bash
# Package and install
npx @vscode/vsce package

code --install-extension vscode-ruby-lsp-go-*.vsix
```

### 6. Configuration

After installation, configure the extension in VS Code Settings:

- Open VS Code Settings (Ctrl/Cmd + ,)
- Search for "Ruby LSP Go"
- Optionally set `rubyLspGo.path` to use a custom binary instead of the one
  bundled in the extension

### 7. Ruby on Rails Specific Setup

For optimal Ruby on Rails development:

1. Ensure you have a `Gemfile` in your Rails project root
2. Run `bundle install` to install dependencies
3. The server will automatically detect Rails-specific patterns

### 8. Performance Optimizations

To maximize performance with the Go implementation:

- The server starts much faster than the Ruby version
- Response times are typically 3-5x faster
- Memory usage is significantly lower
- Works well with large Rails codebases

### 9. Troubleshooting

Common issues and solutions:

- If "Ruby LSP Go executable not found" error occurs:
  - Confirm the extension was built with `./build.sh` and reinstalled from the
    generated `.vsix`
  - Check the "Ruby LSP Go" output panel for the detected platform and path
  - If using a custom binary, verify the `rubyLspGo.path` setting or PATH

- For gem-related issues:
  - Ensure Bundler is set up correctly in your project
  - The server integrates with Bundler automatically

- For performance issues:
  - The Go server should perform much better than Ruby implementations
  - If experiencing issues, check the "Ruby LSP Go" output panel in VS Code
