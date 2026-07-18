# Contributing to P2P Version Control

Thank you for your interest in contributing! This document outlines the process
for contributing to the project.

---

## Code of Conduct

This project adheres to the [Contributor Covenant](CODE_OF_CONDUCT.md) Code of Conduct.
By participating, you are expected to uphold it; please report unacceptable behavior as
described there. In short, we expect all contributors to:

- Be respectful and considerate in all interactions
- Accept constructive criticism gracefully
- Focus on what is best for the community and the project
- Show empathy towards other community members

---

## How to Contribute

### 1. Find Something to Work On

- **Issues:** Look for [open issues](https://github.com/arihant-dev/Action-learning-p2p-version-control/issues)
  labeled `good first issue` or `help wanted`
- **Feature requests:** Check if there's a feature you'd like to implement
- **Bugs:** Report bugs via the [bug report template](.github/ISSUE_TEMPLATE/bug_report.md)

### 2. Discuss Before Implementing

For significant changes (new features, refactoring, major bug fixes),
please open an issue first to discuss the approach. This prevents wasted effort
if the change doesn't align with the project direction.

### 3. Fork and Branch

```bash
git clone <your-fork>
git checkout -b feature/your-feature-name
```

Follow the branch naming convention:
- `feature/<name>` — New features
- `fix/<name>` — Bug fixes
- `docs/<name>` — Documentation changes
- `test/<name>` — Test additions or fixes
- `refactor/<name>` — Code refactoring
- `ws/<name>` — Long-lived workstream branches

### 4. Implement Your Changes

See the [Developer Onboarding Guide](docs/guide/developer-onboarding.md) for setup instructions.

### 5. Test Your Changes

```bash
# Go tests
cd src/backend/go && go test ./... -count=1

# C++ tests
cd src/backend/cpp && ctest --test-dir build --output-on-failure

# Java compilation check
./mvnw compile
```

### 6. Submit a Pull Request

Open a PR against the `master` branch using the [PR template](.github/PULL_REQUEST_TEMPLATE.md).

---

## Development Setup

See the [Developer Onboarding Guide](docs/guide/developer-onboarding.md) for detailed setup
instructions covering prerequisites, build steps, and development workflow.

### Quick Setup

```bash
# Prerequisites
# Go 1.22+, JDK 21+, Maven 3.8+, CMake 3.16+, g++ 11+/clang 14+

cd src/backend/go && go build ./...
cd ../cpp && cmake -B build && cmake --build build
cd ../.. && ./mvnw compile
```

---

## Coding Standards

### Go

- Follow [standard Go conventions](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Run `golangci-lint` before submitting
- Keep functions small and focused (single responsibility)
- Use table-driven tests for test coverage
- Document exported functions with comments

```bash
gofmt -s -w .
golangci-lint run ./...
```

### C++

- Follow the [Google C++ Style Guide](https://google.github.io/styleguide/cppguide.html)
- Use `clang-format` for formatting
- Name files with `.cpp` and `.hpp` extensions
- Use C++20 features (but maintain portability to C++17)
- Prefer header-only library dependencies (nlohmann/json, fmtlib)
- Use RAII for resource and socket management

```bash
clang-format -i src/backend/cpp/src/*.cpp src/backend/cpp/include/*.h
```

### Java

- Follow [Oracle Java conventions](https://www.oracle.com/java/technologies/javase/codeconventions-contents.html)
- Use Java 21 features where appropriate
- Use FXML for layouts, not programmatic UI construction
- Use `JsonObject` for IPC message building
- Keep controllers small; delegate logic to service classes

### General

- No commented-out code in PRs
- No TODOs in code unless linked to an issue
- Follow the existing code style in the file you're modifying
- Keep imports organized (stdlib first, then third-party, then local)

---

## Commit Message Format

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <short description>

[optional body]

[optional footer]
```

### Types

| Type       | Description                              |
| ---------- | ---------------------------------------- |
| `feat`    | A new feature                            |
| `fix`     | A bug fix                                |
| `docs`    | Documentation only changes               |
| `style`   | Formatting, missing whitespace, etc.     |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `perf`    | Performance improvement                  |
| `test`    | Adding or fixing tests                   |
| `ci`      | CI/CD configuration changes             |
| `chore`   | Build process or tooling changes         |

### Examples

```
feat: add delta sync for large file optimization
fix: prevent panic on nil IPC connection
docs: add conflict resolution flow diagram
test: add vector clock concurrent merge test
refactor: extract message validation into dedicated file
ci: add macOS runner to test suite
```

---

## Pull Request Process

1. Ensure your PR passes all CI checks (Go tests, C++ tests, Java compilation)
2. Update documentation if your change affects user-facing behavior or APIs
3. Add or update tests to cover your changes
4. Request review from at least one maintainer
5. Address all review feedback
6. A maintainer merges the PR

### PR Checklist

Before submitting:

- [ ] Code compiles across all three languages
- [ ] Tests pass
- [ ] No linting warnings
- [ ] Documentation updated (if needed)
- [ ] Conventional commit message used
- [ ] Branch is up to date with `master`

---

## Testing Requirements

| Change Type | Tests Required | Minimum Coverage |
|-------------|---------------|-----------------|
| Bug fix | Add test that reproduced the bug | ~100% for the fix |
| New feature | Tests for the new functionality | >80% for new code |
| Refactoring | Existing tests must still pass | No regression |
| Documentation | None | N/A |

### Go Test Conventions

```go
func TestFunctionName(t *testing.T) {
    t.Parallel() // when safe to parallelize
    // Table-driven tests preferred
    tests := []struct {
        name string
        // ...
    }{
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

---

## Review Process

### What Reviewers Look For

1. **Correctness** — Does the code do what it's supposed to do?
2. **Testing** — Are there sufficient tests? Do they cover edge cases?
3. **Documentation** — Are changes documented appropriately?
4. **Performance** — Does the change introduce performance regressions?
5. **Security** — Does the change introduce security vulnerabilities?
6. **Style** — Does the code follow language-specific conventions?

### Review Timeline

- Small PRs (<100 lines): Expected review within 24 hours
- Medium PRs (100-500 lines): Expected review within 48 hours
- Large PRs (>500 lines): May require multiple rounds; consider splitting

---

## Getting Help

- Open an issue for bugs or feature requests
- Tag the relevant maintainer in your PR for review
- For urgent matters, contact the project maintainers

---

## Repository Structure Quick Reference

```
.
├── src/backend/go/       # Go coordinator (networking, coordination)
├── src/backend/cpp/      # C++ watcher daemon (file I/O)
├── src/frontend/         # JavaFX UI
├── docs/                 # Documentation
├── .github/              # CI workflows, templates
├── build_*.sh            # Platform build scripts
└── Dockerfile.linux      # Linux build container
```