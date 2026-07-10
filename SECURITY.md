# Security Policy

## Supported Versions

pantograph is released as a rolling `main` branch; there are no tagged releases.
Only the latest commit on `main` receives security updates. Pin by commit hash to
control when you pick them up (see the [README](README.md#install)).

| Version        | Supported          |
| -------------- | ------------------ |
| latest `main`  | :white_check_mark: |
| older commits  | :x:                |

## Reporting a Vulnerability

Please report security vulnerabilities privately through GitHub's
[**Report a vulnerability**](https://github.com/ilyalavrenov/pantograph/security/advisories/new)
button (the Security tab → Report a vulnerability). This opens a private advisory
so the issue can be handled before public disclosure. Please do not open a public
issue for a suspected vulnerability.

Include enough to reproduce it: the input, the command you ran, and what happened.
You can expect an initial response within a few days; if a report is accepted, a
fix will be released on `main` and credited in the advisory.
