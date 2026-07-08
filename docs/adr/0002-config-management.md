# ADR-0002: Configuration Management System

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
All configuration is currently managed through environment variables with hardcoded defaults scattered across the codebase. There is no config file support, no validation of configuration values, and no hierarchy for overriding settings. This makes deployment inflexible and configuration errors hard to diagnose.

## Decision
Adopt a layered configuration system using YAML config files and the viper library:

- **Precedence (highest to lowest):** CLI flags → environment variables → config file → hardcoded defaults
- **Config file location:** `~/.p2p-vc/config.yaml` with optional `--config` flag override
- **Format:** YAML for human readability, with support for comments via a separate schema documentation
- **Validation:** All config values are validated at startup with clear error messages
- **Reload:** Config is loaded once at startup; hot-reload is deferred to a future iteration

## Consequences
- **Positive:** Backward compatible — existing env-var-based deployments continue to work; flexible deployment options (CLI for one-off overrides, file for persistent config); validation catches misconfiguration early
- **Negative:** Slight increase in startup complexity; additional library dependency (viper)
- **Risks:** YAML parsing quirks (tab vs spaces); secret values in config files must be handled via env vars

## Alternatives Considered
- **TOML:** More rigid, less ecosystem support in Go; no built-in env var binding in viper
- **JSON:** No comments, less readable for humans; every change requires careful comma management
- **Env-only:** Too limited for complex deployments; no way to persist non-default settings; no validation at source
