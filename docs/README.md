# Scripthold Documentation

Start with the [project README](../README.md) to install Scripthold and connect an MCP client. Use this page to find the right guide without reading the entire reference library.

## Use Scripthold

| I want to… | Start here |
|---|---|
| Install a release, build from source, or connect through stdio | [Quick start](../README.md#quick-start) |
| Adopt, update, check, or recover a standalone binary | [Verified Self-update](SELF_UPDATE.md) |
| Find an MCP tool, parameter, example, or error code | [Tool Reference](../TOOLS.md) |
| Understand the supported source languages and analysis capabilities | [Language Capabilities](LANGUAGE_CAPABILITIES.md) |
| Inspect or safely edit Markdown documents | [Markdown Intelligence](MARKDOWN_INTELLIGENCE.md) and the [Markdown tools](../TOOLS.md#markdown_read) |
| Configure tasks that outlive an MCP request | [Durable Tasks](DURABLE_TASKS.md) |
| Run the server over HTTP or through a trusted proxy | [HTTP Security](HTTP_SECURITY.md) |
| Understand how filesystem permissions, encodings, backups, and mutations work | [Architecture](ARCHITECTURE.md) |
| Update a client or integration from an older release | [Migration from 1.8 to 2.0](MIGRATION_2.0.md) or [Migration to 3.0](MIGRATION_3.0.md) |
| See what changed between releases | [Changelog](../CHANGELOG.md) |

## Build, secure, and maintain Scripthold

| I want to… | Start here |
|---|---|
| Understand the project's scope and upstream relationship | [Project Direction](PROJECT_DIRECTION.md) |
| Find planned work and compatibility boundaries | [Roadmap](ROADMAP.md) |
| Submit a contribution and run the right tests | [Contributing](../CONTRIBUTING.md) and [Development Checklist](DEVELOPMENT_CHECKLIST.md) |
| Report a security vulnerability privately | [Security Policy](../SECURITY.md) |
| Prepare, verify, or publish a release | [Publishing Procedure](PUBLISHING.md) |

## How these guides fit together

This page is a navigation aid, not a replacement for the detailed contracts. The [Tool Reference](../TOOLS.md) defines tool behavior; [Architecture](ARCHITECTURE.md) and the focused security/task guides define operational boundaries. Keep release history in the [Changelog](../CHANGELOG.md) and future work in the [Roadmap](ROADMAP.md) rather than repeating either elsewhere.

The [Language Capabilities](LANGUAGE_CAPABILITIES.md) matrix is generated from the source-provider registry, not edited by hand. Migration guides describe their named historical versions, which may differ from today's release. Repository [AGENTS.md](../AGENTS.md) files contain contributor instructions, not end-user guidance.
