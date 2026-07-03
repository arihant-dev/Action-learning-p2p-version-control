#!/bin/bash
set -e

echo "===================================================="
echo " Starting P2P Version Control Build Inside Docker"
echo " Target: Linux (x86_64 / amd64)"
echo "===================================================="

# Compile Go coordinator
echo "--> 1. Compiling Go Coordinator..."
mkdir -p build
cd src/backend/go
go build -o ../../../build/go_coordinator main.go
cd /workspace

# Compile C++ daemon
echo "--> 2. Compiling C++ Watcher Daemon..."
rm -rf src/backend/cpp/build
mkdir -p src/backend/cpp/build
cd src/backend/cpp/build
cmake -DCMAKE_BUILD_TYPE=Release ..
make -j$(nproc 2>/dev/null || echo 4)
cd /workspace

# Build JavaFX runtime
echo "--> 3. Compiling JavaFX & Packaging JLink Runtime..."
mvn clean javafx:jlink

# Copy Go and C++ components
echo "--> 4. Embedding Go & C++ components in Linux bundle..."
cp build/go_coordinator target/app/bin/
cp src/backend/cpp/build/bin/cpp_daemon target/app/bin/

# Package using jpackage
echo "--> 5. Generating Linux App Image..."
rm -rf target/bundle
jpackage --type app-image --name "P2PVersionControl" --runtime-image target/app --module org.codehaus.mojo.frontendtest/org.codehaus.mojo.frontendtest.HelloApplication --dest target/bundle --verbose

# Create a tarball of the final self-contained app
echo "--> 6. Creating final distribution tarball..."
tar -czf target/P2PVersionControl-linux-x64.tar.gz -C target/bundle P2PVersionControl

echo "===================================================="
echo " Build Success!"
echo " Output Archive: target/P2PVersionControl-linux-x64.tar.gz"
echo "===================================================="
