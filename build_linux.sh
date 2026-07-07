#!/bin/bash
set -e

VERSION="${VERSION:-1.0.1}"
APP_NAME="P2PVersionControl"

echo "===================================================="
echo " Building $APP_NAME for Linux (v$VERSION)"
echo "===================================================="

# Check dependencies
dependencies=("go" "cmake" "make" "g++")
missing=0

for dep in "${dependencies[@]}"; do
    if ! command -v "$dep" &> /dev/null; then
        echo "Error: Required dependency '$dep' is not installed."
        missing=1
    fi
done

if [ $missing -eq 1 ]; then
    echo "Please install the missing dependencies and run this script again."
    exit 1
fi

echo "--> 1. Compiling Go Coordinator..."
mkdir -p build
(
    cd src/backend/go
    go build -o ../../../build/go_coordinator main.go
)

echo "--> 2. Compiling C++ Watcher Daemon..."
mkdir -p src/backend/cpp/build
(
    cd src/backend/cpp/build
    cmake -DCMAKE_BUILD_TYPE=Release ..
    make -j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)
)

echo "--> 3. Compiling JavaFX Frontend & Packaging Runtime..."
if [ -f "./mvnw" ]; then
    ./mvnw clean javafx:jlink
else
    mvn clean javafx:jlink
fi

echo "--> 4. Embedding Go & C++ components in App Bundle..."
mkdir -p target/app/bin
cp build/go_coordinator target/app/bin/
cp src/backend/cpp/build/bin/cpp_daemon target/app/bin/

echo "--> 5. Generating Self-Contained App Image..."
rm -rf target/bundle
jpackage \
    --type app-image \
    --name "$APP_NAME" \
    --app-version "$VERSION" \
    --runtime-image target/app \
    --module org.codehaus.mojo.frontendtest/org.codehaus.mojo.frontendtest.HelloApplication \
    --dest target/bundle \
    --verbose

echo "--> 6. Creating distribution tarball..."
(
    cd target/bundle
    tar -czf "../${APP_NAME}-${VERSION}-linux-x64.tar.gz" "$APP_NAME"
)

echo "===================================================="
echo " Build Success!"
echo "   Version: $VERSION"
echo "   App Image:  target/bundle/$APP_NAME"
echo "   Archive:    target/${APP_NAME}-${VERSION}-linux-x64.tar.gz"
echo " Launch with: ./target/bundle/$APP_NAME/bin/$APP_NAME"
echo "===================================================="
