# ADR-0005: Windows IPC Strategy — Named Pipes

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
The IPC mechanism between the Go coordinator and C++ daemon currently uses Unix domain sockets. This is a POSIX-only feature, making it impossible to run the daemon natively on Windows. Supporting Windows is a hard requirement for enterprise deployment.

## Decision
Implement a platform-abstracted IPC interface with conditionally compiled implementations:

- **Unix (Linux/macOS):** Unix domain sockets (existing implementation, `AF_UNIX` + `SOCK_STREAM`)
- **Windows:** Named pipes (`\\.\pipe\P2PVC`) using `CreateNamedPipe` + `ConnectNamedPipe` for the server side, `CreateFile` for the client side, overlapped I/O for async operations
- **Interface:** Abstract `IpcChannel` class in C++ / `IpcConnector` interface in Go with identical semantics (connect, send, receive, close)
- **Message format:** Same length-prefixed protobuf framing on both platforms
- **Build:** Conditional compilation with `#ifdef _WIN32` in C++ and `//go:build windows` tags in Go

## Consequences
- **Positive:** Full native Windows support without WSL or Cygwin; named pipes are the idiomatic Windows IPC mechanism; shared code for message serialization/deserialization reduces duplication
- **Negative:** More code to maintain (two IPC implementations); named pipes have different semantics than Unix sockets (e.g., byte mode vs message mode); conditional compilation adds build complexity
- **Risks:** Named pipe impersonation security considerations; different maximum message size limits; path length limits on pipe names

## Alternatives Considered
- **TCP always:** Works everywhere but is slower (full network stack), less secure (no filesystem ACLs on the socket), and requires port management
- **Cygwin:** Provides Unix domain sockets on Windows but adds a heavy dependency and DLL requirements; not a native Windows experience
- **WSL (Windows Subsystem for Linux):** Not suitable for a production desktop application; requires WSL to be installed and configured by the user
