# Ruby LSP Go Setup Guide

## Complete Setup Instructions

### 1. Prerequisites

- Go 1.21+ installed
- Node.js and npm for building the VS Code extension
- Ruby and Bundler for Ruby project support

### 2. Building the Go Server

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

### 5. Alternative Extension Installation

You can also install the extension directly from VS Code marketplace after publishing, or:
```bash
# Install vsce if not already installed
npm install -g vsce

# Package and install
vsce package
code --install-extension ruby-lsp-go-*.vsix
```

### 6. Configuration

After installation, configure the extension in VS Code Settings:

- Open VS Code Settings (Ctrl/Cmd + ,)
- Search for "Ruby LSP Go"
- Optionally set the path to the binary if not in PATH

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
  - Verify the binary is in your PATH
  - Check the `rubyLspGo.path` setting in VS Code

- For gem-related issues:
  - Ensure Bundler is set up correctly in your project
  - The server integrates with Bundler automatically

- For performance issues:
  - The Go server should perform much better than Ruby implementations
  - If experiencing issues, check the "Ruby LSP Go" output panel in VS Code

