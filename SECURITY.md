# Security Policy

## Supported Versions

We provide security updates for the following versions of Gorest:

| Version | Supported          | Status |
| ------- | ------------------ | ------ |
| 2.0.x   | :white_check_mark: | Current major release |
| 1.0.x   | :warning: | Security fixes only until 2025-06-01 |
| 0.x.x   | :x: | No longer supported |

## Reporting a Vulnerability

If you discover a security vulnerability within Gorest, please send an email to [rathodveerender25@gmail.com](mailto:rathodveerender25@gmail.com). All security vulnerabilities will be promptly addressed.

Please include the following information (if applicable):

- Type of issue (e.g. buffer overflow, SQL injection, cross-site scripting, etc.)
- Full paths of source file(s) related to the manifestation of the issue
- The location of the affected source code (tag/branch/commit or direct URL)
- Any special configuration required to reproduce the issue
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit the issue

This information will help us triage your report more quickly.

## Security Features in v2.0

GoRest v2.0 includes several built-in security features:

- **Request Timeout Protection**: Configurable timeouts prevent resource exhaustion
- **Rate Limiting**: Built-in protection against abuse and DoS attacks
- **Circuit Breaker**: Prevents cascade failures and service overload
- **Context Cancellation**: Proper cleanup of resources on request cancellation
- **Connection Pooling**: Limits concurrent connections to prevent resource exhaustion
- **Structured Logging**: Security-relevant events are logged for monitoring

## Disclosure Policy

When we receive a security bug report, we will:

1. Confirm the problem and determine the affected versions.
2. Audit code to find any potential similar problems.
3. Prepare fixes for all active supported versions.
4. Release security patches as soon as possible.
