#!/bin/bash
set -e

echo "Building lfg..."
go build -o lfg .

echo "Installing to /usr/local/bin..."
sudo mv lfg /usr/local/bin/lfg

echo "✓ lfg installed successfully!"
echo "Run 'lfg' to get started."
