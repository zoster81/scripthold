# Verified self-update

Use the `scripthold update` commands to update a **supported standalone binary** through a verified release workflow. This is an opt-in feature: adoption prepares the installation for updates, and a later update checks release evidence before attempting a replacement. MCPB-managed and other unsupported installations cannot use this workflow.

## 1. Adopt a standalone installation

Before adopting, stop every other Scripthold process that uses the same binary. Keep those processes stopped until adoption has finished.

```text
scripthold update --adopt
```

Adoption checks the installed binary and prepares local, durable self-update state. It **does not** contact the network, download a release, or replace the executable. MCPB-managed, linked or aliased, unsupported, dirty, or already-adopted installations are rejected rather than updated unsafely.

## 2. Check and dispatch an update

After successful adoption, use:

```text
scripthold update
```

Scripthold checks for a newer immutable release, verifies the matching platform binary and release evidence, stages the candidate, and starts a detached update helper. If no newer release exists, the command succeeds without changing the installation.

**A successful dispatch is not proof that replacement has finished.** Check `scripthold update status` before launching another update or restarting other processes. If the update helper started but launcher cleanup failed, inspect status before retrying.

## 3. Inspect status

```text
scripthold update status
```

Status is read-only and does not start the MCP server. It reports whether the installation is adopted, the observed update state, verified current/candidate version evidence, release availability, and whether explicit recovery is available.

If the installed binary has no verifiable semantic version, Scripthold suppresses the remote version comparison instead of trusting stale information. Set `MCP_NO_UPDATE_CHECK=1` to disable the remote check performed by `update status`; the latest available version will then be reported as unknown.

## 4. Recover only when status offers recovery

```text
scripthold update recover
```

Use recovery only when status reports that recovery is available. It launches a fixed, verified recovery helper, requires durable transaction evidence and the observed installation bytes, and does not contact the network.

Dispatching recovery does not mean recovery is complete. Check status afterward, especially if the launcher reports a cleanup failure after starting the helper.

## A directory named `update`

The command word `update` is reserved. If you need to pass an authorized directory literally named `update` as a normal server argument, use `-- update`.

For broader release provenance and immutable publication requirements, see the [Publishing Procedure](PUBLISHING.md). For internal state and recovery boundaries, see [Architecture](ARCHITECTURE.md#verified-self-update-lifecycle-and-status).
