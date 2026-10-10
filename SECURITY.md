# Security Policy

## Reporting a vulnerability

**Please report security vulnerabilities privately.** Do not open a public
issue for anything exploitable.

Use GitHub's private reporting:
[**Report a vulnerability**](https://github.com/kawaiipantsu/xxc-cleaners/security/advisories/new)

Because `xxc-cleaners` deletes files as root, we are especially interested in:

- Any way to make the tool delete a **protected** path (`/etc`, databases,
  Docker volumes, SSH/GPG keys, boot files, etc.).
- Path-traversal, symlink-following or TOCTOU issues that escape an intended
  cleaning root.
- Config parsing or glob handling that broadens deletion beyond what the
  operator asked for.
- Leakage of the APT deploy key or other secrets.

When reporting, please include the version (`xxc-cleaners --version`), the
platform, and a minimal description. Avoid sending working destructive
exploits against systems you do not own.

## Supported versions

The latest released version receives security fixes.

## Disclosure

We aim to acknowledge reports quickly and coordinate a fix and disclosure
timeline with you. Credit is given unless you prefer to remain anonymous.

— Kawaiipantsu · [THUGS(red)](https://thugs.red)
