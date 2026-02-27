# Ruby LSP Go

A high-performance Ruby Language Server Protocol (LSP) implementation in Go, inspired by the Shopify Ruby LSP project.

## Overview

This project is a Go implementation of the Ruby Language Server Protocol server, designed to provide faster performance than the original Ruby implementation. It offers IDE features like:

- Syntax highlighting
- Code completion
- Hover documentation
- Go to definition
- Document symbols
- Code formatting
- Semantic highlighting

## Features

- **Fast**: Written in Go for optimal performance
- **Lightweight**: Minimal resource usage
- **Standard LSP Compliance**: Follows the Language Server Protocol specification
- **Ruby-Specific**: Optimized for Ruby development workflows

## Architecture

The implementation follows the same architectural patterns as the original Ruby LSP:

- **Message Processing**: Handles LSP communication via stdin/stdout
- **Document Store**: Manages open document states
- **AST Parsing**: Parses Ruby code for intelligent features
- **Request Handling**: Processes various LSP requests (completion, hover, etc.)

## Performance Benefits

Compared to the Ruby implementation:

- Faster startup times
- Quicker response times for requests
- More efficient memory usage
- Better concurrent processing capabilities

## Implementation Notes

This Go implementation maintains compatibility with the original Ruby LSP while providing substantial performance improvements. Key components include:

- Robust JSON-RPC message handling
- Efficient document management
- Basic Ruby AST parsing for core features
- Standard LSP capabilities

## Usage

To run the server directly:
\`\`\`
go run main.go
\`\`\`

The server communicates over stdin/stdout as per the LSP specification.

## Future Enhancements

Potential improvements include:
- Integration with Ruby parsers like Prism
- Advanced code analysis features
- Enhanced semantic understanding
- Better integration with Ruby ecosystem tools

