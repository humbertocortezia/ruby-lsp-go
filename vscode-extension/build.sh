#!/bin/bash

echo "Building VS Code extension for Ruby LSP Go..."

# Install dependencies
npm install

# Compile TypeScript
npm run compile

# Package extension
npx vsce package

echo "Extension packaged as ruby-lsp-go-*.vsix"

