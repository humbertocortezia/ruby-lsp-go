# Ruby LSP Go for Visual Studio Code

High-performance Ruby Language Server Protocol implementation in Go.

## Features

This extension provides advanced Ruby language support using a Go implementation of the Ruby LSP server. It includes:

- **IntelliSense**: Smart code completion
- **Hover Information**: Quick documentation on hover
- **Go to Definition**: Navigate to symbol definitions
- **Find All References**: Locate all uses of a symbol
- **Document Symbols**: Outline view of your Ruby files
- **Code Formatting**: Automatic code formatting
- **Diagnostics**: Real-time error detection
- **Code Actions**: Quick fixes and refactorings

## Requirements

- **Ruby LSP Go executable**: Install the Ruby LSP Go server binary
- **Ruby Environment**: A working Ruby installation with Bundler

## Installation

1. Install the Ruby LSP Go server binary:
   ```bash
   # Build from the Go implementation
   cd ruby-lsp-go
   go build -o ruby-lsp-go main.go
   # Make sure ruby-lsp-go is in your PATH
   ```

2. Install this extension in VS Code

## Configuration

The following settings are available:

- `rubyLspGo.path`: Path to the Ruby LSP Go executable
- `rubyLspGo.useBundler`: Whether to run with bundle exec (default: true)
- `rubyLspGo.formatter`: Code formatter to use (auto, none, rubocop, syntax_tree)
- `rubyLspGo.linters`: Array of linters to use
- `rubyLspGo.enabledFeatures`: Object to enable/disable specific LSP features

## Ruby on Rails Support

The extension fully supports Ruby on Rails projects:

- Model associations navigation
- Controller and view references
- Migration and schema support
- Routing and helpers
- Gems integration

## Commands

- `Ruby LSP Go: Restart Server` - Restarts the language server

## Troubleshooting

If you encounter issues:

1. Check the "Ruby LSP Go" output panel for error messages
2. Verify the Ruby LSP Go executable is accessible
3. Ensure your project has a Gemfile and is properly configured

## Performance Benefits

The Go implementation provides:

- Faster startup times
- Quicker response times
- Better memory efficiency
- Improved concurrent processing

Compared to the Ruby implementation, this Go version delivers significantly better performance for large Ruby and Rails codebases.

