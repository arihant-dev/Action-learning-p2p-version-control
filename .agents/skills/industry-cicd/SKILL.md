---
name: industry-cicd
description: Multi-platform CI/CD, pre-commit hooks, release automation, and Docker images.
---

# CI/CD Agent

**Role:** Build reliable multi-platform CI/CD pipeline, pre-commit checks, and release automation
**Branch:** `ws/cicd` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 2 (depends on Testing, Architecture)

## Work Items (in priority order)

1. **Windows CI Runner**
   - Add `windows-latest` to GitHub Actions test matrix
   - Install: Go, CMake, Visual Studio Build Tools, JDK 21, Maven
   - Run: Go tests, C++ CMake build, Java compile check
   - Handle path separator differences in tests

2. **Cross-Platform Build Matrix**
   - Expand release workflow:
     - `ubuntu-latest` (amd64 + arm64 via buildx)
     - `macos-latest` (arm64: macos-15-xlarge, amd64: macos-13)
     - `windows-latest` (amd64)
   - Build artifacts: `.dmg` (macOS), `.msi`/`.exe` (Windows), `.tar.gz` (Linux), `.deb` (Linux)

3. **Pre-commit Hooks**
   - Configure `.pre-commit-config.yaml`:
     - `golangci-lint` for Go (with config in `.golangci.yml`)
     - `clang-format` for C++ (with `.clang-format` config)
     - `spotless` or `checkstyle` for Java
     - `yaml-lint` for YAML files
     - `prettier` for markdown/JSON
   - Auto-fix where possible

4. **Integration Test Workflow**
   - New GitHub Actions workflow: `integration.yml`
   - Steps:
     1. Build Go coordinator
     2. Build C++ daemon
     3. Build Java frontend
     4. Start Go coordinator in background
     5. Start C++ daemon for a test repo
     6. Run Python E2E harness
     7. Collect logs on failure
   - Run on push to `industry-shipping` branch

5. **Semantic Versioning Automation**
   - Use `semantic-release` or `go-semrel` for automatic versioning
   - Commit message format: `feat:` (minor), `fix:` (patch), `BREAKING CHANGE:` (major)
   - Generate CHANGELOG.md from conventional commits
   - Tag releases automatically on merge to `industry-shipping`

6. **Docker Images for Linux Deployment**
   - Create `Dockerfile.release` for production runtime (not build):
     - Multi-stage: runtime based on `alpine:3.20` or `debian:stable-slim`
     - Copy pre-built Go binary, C++ daemon, JavaFX jlink image
     - Entrypoint: start Go coordinator
   - Publish to GitHub Container Registry on release

7. **SBOM Generation**
   - Add SBOM (Software Bill of Materials) generation to release workflow
   - Use `syft` or `cyclonedx-maven-plugin`
   - Capture: Go module dependencies, Maven dependencies, C++ library dependencies
   - Attach SBOM to release artifacts

8. **Upgrade/Downgrade Test**
   - Integration test that verifies:
     - v1.0.0 peer can sync with v1.1.0 peer (backward compat)
     - Downgrade from v1.1.0 to v1.0.0 does not corrupt data
   - Add compatibility test workflow

## Relevant Files
- `.github/workflows/testing.yml` — CI pipeline
- `.github/workflows/release.yml` — Release pipeline
- `Dockerfile.linux` — Build container (refactor to separate runtime)
- `build_macos.sh`, `build_linux.sh` — Build scripts
- `pom.xml` — Add cyclonedx plugin
- `src/backend/go/go.mod` — Already has dependencies

## Verification
- All workflows pass on all platforms (Linux, macOS, Windows)
- Pre-commit hooks run successfully on all file types
- `semantic-release --dry-run` shows correct version bump
- Docker image starts and Go health endpoint responds
- SBOM is valid CycloneDX/SPDX format
