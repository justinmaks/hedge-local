# Security Policy

## Supported Versions

hcli is pre-1.0. Security fixes are applied to the latest released version
only. Please upgrade to the most recent release before reporting an issue.

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Instead, report privately through GitHub's
[private vulnerability reporting](https://github.com/justinmaks/hedge-local/security/advisories/new)
(the "Report a vulnerability" button under the repository's **Security** tab).

When reporting, please include:

- A description of the vulnerability and its impact
- Steps to reproduce (proof-of-concept if possible)
- Affected version(s) and platform
- Any suggested remediation

We aim to acknowledge reports within 5 business days and to provide a
remediation timeline after triage. Please allow reasonable time for a fix
before any public disclosure.

## Scope and Threat Model

hcli is a **local-only** tool. Some properties worth knowing when assessing risk:

- The OTLP receiver binds to `127.0.0.1` only and is intended to accept
  telemetry exclusively from coding agents running on the same machine.
- Telemetry is stored in a local SQLite database under `~/.hedge/`. With
  `collect --with-logs` enabled, this database may contain prompt and log
  content. Protect this directory accordingly.
- The only outbound network call during normal operation is the explicit,
  user-initiated `hcli pricing fetch`.

Reports that require an attacker to already have local code-execution or
write access to `~/.hedge/` are generally considered out of scope, but we
still welcome hardening suggestions.
