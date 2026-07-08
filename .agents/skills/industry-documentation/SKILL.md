---
name: industry-documentation
description: Comprehensive documentation — ADRs, API specs, deployment guides, user manual.
---

# Documentation Agent

**Role:** Create comprehensive documentation suite for developers, operators, and end users
**Branch:** `ws/documentation` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 1

## Work Items (in priority order)

1. **Architecture Decision Records (ADRs)**
   - Create `docs/adr/` directory with template
   - ADR-001: TLS Mutual Authentication for P2P Communication
   - ADR-002: Configuration File Format and Precedence (TOML/YAML)
   - ADR-003: Structured Logging Strategy (zerolog JSON format)
   - ADR-004: Native Filesystem Watchers Platform Abstraction
   - ADR-005: Windows Port — Named Pipes vs Unix Sockets
   - ADR-006: File Transfer Chunking and Resume Protocol
   - ADR-007: Delta Sync Strategy (rsync-style rolling hash)
   - ADR-008: Conflict Resolution UI Design

2. **IPC Protocol Specification (OpenAPI/AsyncAPI)**
   - Document the framed JSON IPC protocol
   - Specify all message types, payloads, and direction
   - Include example messages for each type
   - Use AsyncAPI spec (YAML) for event-driven IPC

3. **P2P Protocol Specification**
   - Document the TCP-level P2P protocol
   - Handshake, heartbeat, metadata exchange, file transfer
   - Protocol versioning and backward compatibility policy
   - Error codes and recovery procedures

4. **Developer Onboarding Guide**
   - Repository structure overview
   - Prerequisites and setup (Go, JDK, Maven, CMake, compiler)
   - Quick start: build and run locally
   - Development workflow: branches, testing, pre-commit
   - How to add a new IPC message type
   - How to add a new P2P message type
   - Debugging tips (logs, pprof, temp files)

5. **Deployment Guide**
   - Single-node deployment (desktop app)
   - Multi-node deployment (team/office)
   - Docker deployment
   - Configuration reference
   - Security checklist (TLS, certificates, firewall rules)
   - Backup and recovery (SQLite backup)
   - Monitoring (Prometheus metrics, log aggregation)

6. **Security Model Documentation**
   - Threat model: what are we protecting against?
   - Network security: mTLS, certificate management
   - Data security: encryption at rest, file permissions
   - Identity: peer verification, trust on first use (TOFU)
   - Audit: what events are logged and why
   - Incident response: what to do if a peer is compromised

7. **User Manual**
   - Getting started guide (with screenshots for each step)
   - Adding and managing repositories
   - Understanding sync status
   - Resolving conflicts
   - Managing peers
   - Settings and configuration
   - Troubleshooting common issues
   - FAQ

8. **Contribution Guidelines**
   - `CONTRIBUTING.md`: coding standards, PR process, commit conventions
   - Code of conduct
   - Issue templates (bug report, feature request)
   - Pull request template

## Relevant Files
- `README.md` — Update with links to new documentation
- `docs/` — Documentation directory (create subdirectories as needed)
- `docs/adr/` — ADR documents
- `docs/api/` — API specifications
- `docs/guide/` — Developer and user guides

## Verification
- All ADRs follow a consistent template
- API spec validates against AsyncAPI schema
- Developer guide: a new developer can build and run from scratch
- User manual: a non-technical user can set up and use the app
- All links in documentation resolve correctly
