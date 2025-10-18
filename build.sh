#!/bin/bash

# Build script for lfg

set -e  # Exit on error

echo "Building lfg..."
go build -o lfg

echo "Creating symlink..."
ln -sf "$(pwd)/lfg" ~/.local/bin/lfg

echo "✅ Build complete! lfg is now available in your PATH"
echo "Run 'lfg' to start"
