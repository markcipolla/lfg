#!/bin/bash
set -e

echo "Building lfg..."
go build -o lfg

echo "Installing to ~/.local/bin/lfg..."
cp ./lfg /Users/markcipolla/.local/bin/lfg

echo "Done! lfg has been built and installed."
