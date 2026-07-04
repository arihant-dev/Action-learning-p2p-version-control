#!/bin/bash
set -e

echo "===================================================="
echo " Building P2P Version Control for macOS"
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
    echo ""
    echo "  brew install go cmake make gcc"
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
    nproc=$(sysctl -n hw.ncpu 2>/dev/null || echo 4)
    make -j"$nproc"
)

echo "--> 3. Compiling JavaFX Frontend & Packaging Runtime..."
if [ -f "./mvnw" ]; then
    ./mvnw clean javafx:jlink
else
    mvn clean javafx:jlink
fi

echo "--> 4. Embedding Go & C++ components in App Bundle..."
APP_DIR="target/app"
mkdir -p "$APP_DIR/bin"
cp build/go_coordinator "$APP_DIR/bin/"
cp src/backend/cpp/build/bin/cpp_daemon "$APP_DIR/bin/"

echo "--> 5. Generating Self-Contained macOS App Bundle..."
rm -rf target/bundle
jpackage \
    --type app-image \
    --name "P2PVersionControl" \
    --runtime-image target/app \
    --module org.codehaus.mojo.frontendtest/org.codehaus.mojo.frontendtest.HelloApplication \
    --dest target/bundle \
    --verbose

echo "===================================================="
echo " Build Success!"
echo " Launch the app on macOS with:"
echo "   open target/bundle/P2PVersionControl.app"
echo "===================================================="
