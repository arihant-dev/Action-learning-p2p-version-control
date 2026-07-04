#!/bin/bash
set -e

echo "Building Linux amd64 docker image..."
docker build --load --platform linux/amd64 -t p2p-linux-builder -f Dockerfile.linux .

echo "Running Linux build in Docker container..."
chmod +x build_linux_in_docker.sh
docker run --platform linux/amd64 --rm -v "$(pwd)":/workspace p2p-linux-builder:latest /workspace/build_linux_in_docker.sh
