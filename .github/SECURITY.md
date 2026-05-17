# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 1.x (latest) | ✅ Security fixes |
| < 1.0 | ❌ No longer supported |

Only the most recent release receives security patches. Upgrade to the latest version before reporting a vulnerability.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities privately by emailing **casjay@yahoo.com** with the subject line `[SECURITY] wthr — <brief description>`.

Include:
- A description of the vulnerability and its potential impact
- Steps to reproduce (proof-of-concept if available)
- Affected versions
- Any suggested mitigations

**Response SLA:**
- Acknowledgement within **48 hours**
- Patch or mitigation plan within **14 days** for critical/high severity
- Patch or mitigation plan within **30 days** for medium/low severity

## Disclosure Timeline

We follow coordinated disclosure:

1. Reporter submits details privately
2. We acknowledge within 48 hours
3. We investigate and develop a fix
4. We coordinate a release date with the reporter
5. Fix is released; GitHub Security Advisory published after release

Please allow us the agreed timeline before any public disclosure.

## Out of Scope

The following are **not** considered vulnerabilities for this project:

- Vulnerabilities in dependencies that are already tracked upstream (check `govulncheck` output first)
- Rate-limit bypasses via VPN or Tor (GeoIP is a signal only, not an access gate)
- Self-XSS requiring physical access to an authenticated session
- Social-engineering attacks against administrators
- Denial-of-service against a single, unauthenticated instance without amplification
- Issues in unofficial forks or modified deployments

## Security Architecture Notes

- **Admin authentication** is WebAuthn/passkey-only (no password fallback); credential stuffing against the admin panel is not possible
- **All passwords** are hashed with Argon2id (time=1, mem=64 MB, threads=4, keyLen=32)
- **Session tokens** are 32 bytes from `crypto/rand`, stored as SHA-256 hashes; raw tokens are returned once and never logged
- **All weather data sources** are public APIs; no API keys are stored
- **Single binary** with embedded GeoIP database; no external runtime dependencies

> CVE details and technical patch notes are published in [GitHub Security Advisories](https://github.com/casapps/wthr/security/advisories) after the fix is released — never in this file.
