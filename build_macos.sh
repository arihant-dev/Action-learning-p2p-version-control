#!/bin/bash
set -e

VERSION="${VERSION:-1.0.0}"
APP_NAME="P2PVersionControl"

echo "===================================================="
echo " Building $APP_NAME for macOS (v$VERSION)"
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

# Clear any extended attributes on the target/app directory to prevent codesign issues
xattr -cr target/app || true

# Use a temporary directory outside the iCloud-synced workspace to run jpackage,
# which avoids macOS Finder/FileProvider from dynamically injecting disallowed xattrs
# (like com.apple.FinderInfo) during the codesign process.
TMP_BUILD_DIR=$(mktemp -d -t jpackage-build)
trap 'rm -rf "$TMP_BUILD_DIR"' EXIT

jpackage \
    --type app-image \
    --name "$APP_NAME" \
    --app-version "$VERSION" \
    --runtime-image target/app \
    --module org.codehaus.mojo.frontendtest/org.codehaus.mojo.frontendtest.HelloApplication \
    --dest "$TMP_BUILD_DIR" \
    --verbose

mkdir -p target/bundle
cp -R "$TMP_BUILD_DIR/$APP_NAME.app" target/bundle/

echo "--> 6. Creating distribution archive..."
(
    cd target/bundle
    zip -r "../${APP_NAME}-${VERSION}-macos.zip" "$APP_NAME.app"
)

echo "===================================================="
echo " Build Success!"
echo "   Version: $VERSION"
echo "   App Bundle: target/bundle/$APP_NAME.app"
echo "   Archive:    target/${APP_NAME}-${VERSION}-macos.zip"
echo " Launch with: open target/bundle/$APP_NAME.app"
echo "===================================================="
