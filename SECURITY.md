# Security Policy

## Supported versions

Security fixes are evaluated for the current public release and the current `main` development line. Older releases may require upgrading to a maintained version rather than receiving a backport.

## Reporting a vulnerability

Please do not publish exploit details, credentials, sensitive paths, or proof-of-concept payloads in a public issue.

Use GitHub's private vulnerability-reporting or Security Advisory flow for this repository when it is available. If GitHub does not offer a private reporting form, open a minimal public issue that contains no sensitive technical details and asks the maintainer to establish a private channel before sharing the report.

A useful report includes:

- the affected release or commit;
- the security impact and required preconditions;
- a minimal, safe reproduction;
- the expected and observed behavior;
- relevant logs with secrets, private paths, tokens, and personal data removed;
- any proposed mitigation or patch, if available.

## Security-sensitive areas

Reports are especially useful for failures involving allowed-root confinement, symlink/junction/reparse-point handling, mutation and backup integrity, task execution boundaries, HTTP authentication/Host/Origin policy, persistent backup/task stores, encoding safety, source-intelligence authorization, or leakage of source and credentials.

## Safe testing and disclosure

Test only systems and data you own or are authorized to assess. Do not exfiltrate data, disrupt third-party systems, or publish an exploit before a fix and coordinated disclosure are possible.

The maintainer will evaluate reports based on reproducibility, impact, and affected supported versions. No fixed response-time or remediation-time SLA is promised.
