# P2P File Sync - C++ Daemon

Cross-platform file system monitoring daemon written in C++.

## Project Structure

```
src/backend/cpp/
├── CMakeLists.txt                 # CMake build configuration
├── README.md                       # This file
├── include/                        # include file
├── tests/                          # tests
└── src/
    └── main.cpp                    # Main entry point with file watcher interface
```

## Features

- **Cross-platform**: Supports Linux, macOS, and Windows
- **Command-line interface**: Monitor any directory via CLI argument
- **Event logging**: JSON-formatted output for file changes
- **Signal handling**: Graceful shutdown on SIGINT/SIGTERM

## Build Requirements

- **C++20**
- **CMake 3.16+**
- **OpenSSL** (for SHA-256 via EVP API)

### Linux

```bash
sudo apt-get install build-essential cmake libssl-dev
```

### macOS

```bash
# Install Xcode Command Line Tools
xcode-select --install
# OpenSSL is usually available via the system or Homebrew
brew install openssl
```

### Windows

```
Visual Studio 2019+ with C++ development tools
OpenSSL (via vcpkg or pre-built)
```

## Building

### Configure and Build

```bash
cd src/backend/cpp
mkdir build
cd build
cmake .. -DCMAKE_BUILD_TYPE=Release
cmake --build .
```

### Testing (include tests)

```bash
cd src/backend/cpp
mkdir build
cd build
cmake .. -DCMAKE_BUILD_TYPE=Debug -DBUILD_TESTS=ON
cmake --build .
ctest --output-on-failure
```

### Output

The binary will be created at: `build/bin/p2p_daemon`

### Running

```bash
./build/bin/p2p_daemon
```

for seeing the usage.

## Usage

### Basic Usage

```bash
./p2p_daemon <repo_id> <watch_path> [ipc_socket] [--poll-interval <ms>]
```

### Example

```bash
./p2p_daemon project-alpha /path/to/watch /tmp/p2p_sync.sock --poll-interval 500
```

Once running, the daemon will output events like:

```
[C++ Daemon] Starting file watcher on: /path/to/watch
[C++ Daemon] Connecting to IPC server at /tmp/p2p_sync.sock...
```

## Dependencies

### Build-Time

- **OpenSSL** (libcrypto, libssl) — SHA-256 hashing and TLS

### Runtime (system libraries)

- **Linux**: inotify (kernel API, no external library)
- **macOS**: FSEvents (system framework, via CoreServices)
- **Windows**: ReadDirectoryChangesW (Windows API)

### Automatic (via FetchContent, test builds only)

- **GoogleTest** — unit test framework (fetched if not found on system)
