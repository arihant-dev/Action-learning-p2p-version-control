#!/bin/bash
set -e

VERSION="${VERSION:-1.0.0}"
APP_NAME="P2PVersionControl"

echo "===================================================="
echo " Starting $APP_NAME Build Inside Docker"
echo " Target: Linux (x86_64 / amd64) — v$VERSION"
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
mkdir -p target/app/bin
cp build/go_coordinator target/app/bin/
cp src/backend/cpp/build/bin/cpp_daemon target/app/bin/

# Package using jpackage
echo "--> 5. Generating Linux App Image..."
rm -rf target/bundle
jpackage \
    --type app-image \
    --name "$APP_NAME" \
    --app-version "$VERSION" \
    --runtime-image target/app \
    --module org.codehaus.mojo.frontendtest/org.codehaus.mojo.frontendtest.HelloApplication \
    --dest target/bundle \
    --verbose

# Create a tarball of the final self-contained app
echo "--> 6. Creating distribution tarball..."
tar -czf "target/${APP_NAME}-${VERSION}-linux-x64.tar.gz" -C target/bundle "$APP_NAME"

echo "===================================================="
echo " Build Success!"
echo "   Version: $VERSION"
echo "   Archive: target/${APP_NAME}-${VERSION}-linux-x64.tar.gz"
echo "===================================================="
