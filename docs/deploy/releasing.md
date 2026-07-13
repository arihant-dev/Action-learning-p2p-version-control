# Releasing

This document describes how the `.github/workflows/release.yml` pipeline builds,
signs, checksums, and publishes releases of P2P Version Control, and how
maintainers and end users interact with it.

---

## Table of Contents

1. [Release Flow Overview](#1-release-flow-overview)
2. [GitHub Secrets Reference](#2-github-secrets-reference)
3. [Setting Up Secrets (Maintainers)](#3-setting-up-secrets-maintainers)
4. [Verifying Downloads (End Users)](#4-verifying-downloads-end-users)
5. [Unsigned Builds](#5-unsigned-builds)

---

## 1. Release Flow Overview

The pipeline is triggered by:

- Pushing a tag matching `v*` (e.g. `v1.5.0`), or
- Manually via `workflow_dispatch`, with an optional `dry_run` flag that skips
  the final GitHub Release creation step so the build can be exercised without
  publishing anything.

High-level flow:

1. **Build** — three parallel jobs build the app for macOS (Intel + Apple
   Silicon), Linux, and Windows using the Go coordinator, C++ watcher daemon,
   and JavaFX/jpackage frontend.
2. **Sign (conditional)** — if the relevant secrets are configured, each
   platform's artifact is signed:
   - macOS: the `.app` bundle is codesigned, notarized with Apple's
     `notarytool`, and the notarization ticket is stapled to the bundle before
     it is re-archived into the `.zip` that gets uploaded.
   - Windows: the `.msi` installer is signed with Authenticode (`signtool`)
     using a timestamp server.
   - If the signing secrets are **not** configured, these steps are skipped
     and the build proceeds with unsigned artifacts — the pipeline never
     fails because of missing signing credentials.
3. **Checksum (always)** — once all platform artifacts (and the CycloneDX
   SBOM) are downloaded into the `create-release` job, a `SHA256SUMS` file is
   generated covering every artifact. This step is unconditional and runs
   regardless of whether signing secrets are present.
4. **GPG sign checksums (conditional)** — if a GPG signing key is configured,
   a detached signature `SHA256SUMS.asc` is produced for `SHA256SUMS`.
5. **Publish** — a GitHub Release is created (unless `dry_run` is set) with
   all platform artifacts, the SBOM, `SHA256SUMS`, and `SHA256SUMS.asc` (if
   produced) attached.

## 2. GitHub Secrets Reference

All secrets are read exclusively via the `${{ secrets.* }}` context inside
`.github/workflows/release.yml`. None are required — every signing stage is
conditional and the pipeline produces valid unsigned artifacts if a secret is
absent.

### macOS code signing & notarization

| Secret | Purpose |
|---|---|
| `MACOS_CERTIFICATE_P12_BASE64` | Base64-encoded `.p12` export of a Developer ID Application certificate + private key. Presence of this secret is what turns `HAS_MACOS_SIGN` on. |
| `MACOS_CERTIFICATE_PWD` | Password protecting the `.p12` file above. |
| `MACOS_SIGN_IDENTITY` | The signing identity string passed to `codesign --sign`, e.g. `Developer ID Application: Your Org (TEAMID)`. |
| `APPLE_ID` | Apple ID email used to authenticate with `notarytool`. |
| `APPLE_APP_PASSWORD` | An [app-specific password](https://support.apple.com/en-us/102654) for the Apple ID above (not the main account password). |
| `APPLE_TEAM_ID` | Your Apple Developer Team ID. |

### Windows Authenticode signing

| Secret | Purpose |
|---|---|
| `WINDOWS_CERT_PFX_BASE64` | Base64-encoded `.pfx` code-signing certificate. Presence of this secret is what turns `HAS_WINDOWS_SIGN` on. |
| `WINDOWS_CERT_PWD` | Password protecting the `.pfx` file above. |

### Checksum signing (optional, any platform)

| Secret | Purpose |
|---|---|
| `GPG_PRIVATE_KEY` | Armored GPG private key used to produce a detached signature of `SHA256SUMS`. Presence of this secret is what turns `HAS_GPG_SIGN` on. |
| `GPG_PASSPHRASE` | Passphrase protecting the GPG private key above. |

`GITHUB_TOKEN` is provided automatically by GitHub Actions and is used only to
create the release via `gh release create`; it is not a secret you need to
configure.

## 3. Setting Up Secrets (Maintainers)

1. Obtain the relevant certificates/keys from your Apple Developer account,
   Windows code-signing CA, and/or GPG keypair. **Never commit these to the
   repository.**
2. Base64-encode binary certificate files before storing them as secrets,
   e.g.:

   ```bash
   base64 -i DeveloperIDApplication.p12 | pbcopy   # macOS
   base64 -w0 codesign.pfx                          # Windows cert, on Linux/macOS
   ```

3. In the GitHub repository, go to **Settings → Secrets and variables →
   Actions → New repository secret** and add each secret name from the
   tables above with its corresponding value.
4. You do not need to configure all of them — configure only the platform(s)
   you intend to sign for. Any missing secret group simply results in that
   platform's artifact being shipped unsigned (or unsigned checksums, in the
   GPG case).
5. Trigger a release (tag push or `workflow_dispatch`) and check the job logs
   for `HAS_MACOS_SIGN`, `HAS_WINDOWS_SIGN`, and `HAS_GPG_SIGN` to confirm
   which signing paths were taken. Secret values themselves are never
   printed.

## 4. Verifying Downloads (End Users)

Every release includes a `SHA256SUMS` file listing the checksum of every
artifact in that release, including the SBOM.

1. Download the artifact(s) you want and `SHA256SUMS` from the release page
   into the same directory.
2. Verify:

   ```bash
   shasum -a 256 -c SHA256SUMS
   ```

   Each file should report `OK`.

If the release was also GPG-signed, `SHA256SUMS.asc` will be present. To
verify the checksum file itself hasn't been tampered with:

```bash
gpg --verify SHA256SUMS.asc SHA256SUMS
```

(You'll need the maintainer's public GPG key imported first —
`gpg --import maintainer-public-key.asc`.)

## 5. Unsigned Builds

If the signing secrets described above are not configured in the repository
(for example, in a fork or before a maintainer has set up certificates), the
release pipeline still runs to completion and produces fully functional,
**unsigned** artifacts for every platform:

- macOS `.app`/`.zip` will not be codesigned or notarized. Users may need to
  right-click → Open, or run `xattr -cr` / adjust Gatekeeper settings, to
  launch the app.
- The Windows `.msi` will not carry an Authenticode signature. Users may see
  an "Unknown publisher" SmartScreen warning.
- `SHA256SUMS` is **always** generated regardless of signing status, so users
  can still verify artifact integrity even for unsigned builds.
- No `SHA256SUMS.asc` is produced if the GPG secret is absent.

This is intentional: missing signing credentials must never break the build
or block a release.
