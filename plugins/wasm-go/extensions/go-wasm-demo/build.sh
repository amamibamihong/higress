#!/bin/bash

set -e

echo "Building presidio-pii Wasm plugin..."

GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm .

if [ $? -eq 0 ]; then
    echo "Build successful! Output: main.wasm"
    ls -lh main.wasm
else
    echo "Build failed!"
    exit 1
fi
