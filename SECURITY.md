# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| latest  | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in Granit, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please email: **raphael.lugmayr@stoicera.com**

Include:
- A description of the vulnerability
- Steps to reproduce the issue
- Potential impact assessment
- Suggested fix (if any)

## Response Timeline

- **Acknowledgment**: Within 48 hours
- **Assessment**: Within 7 days
- **Fix & Disclosure**: Coordinated with reporter, typically within 30 days

## Scope

The following are in scope:
- Command injection via plugin execution
- Path traversal in vault operations
- Credential leakage (API keys, tokens)
- Arbitrary code execution
- Denial of service via crafted input files

The following are out of scope:
- Issues requiring physical access to the machine
- Social engineering attacks
- Vulnerabilities in third-party dependencies (report upstream)

## Security Design

Granit follows these security principles:
- **No telemetry**: Zero network calls unless explicitly configured (AI providers)
- **Minimal dependencies**: Only 5 direct Go dependencies
- **Plugin sandbox**: 10-second execution timeout, path-escape detection
- **No unsafe package**: Memory-safe Go throughout
- **Local-first**: All data stays on your filesystem
