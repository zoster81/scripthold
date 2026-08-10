# Durable task execution

Scripthold's 2.1.x task subsystem replaces the request-bound `run_script` and `shell` tools with durable asynchronous execution. A call to `task_run` validates and records work, then returns a `taskId`; it never waits for the command to finish. This prevents an MCP request timeout, tunnel TTL, stdio EOF, HTTP disconnect, or frontend restart from terminating a long build.

## Process topology

One private task store is shared by authorized stdio and HTTP frontends independently of later allowed-directory configuration changes:

```text
stdio frontend ----\
                    +--> owner-only task store <-- task-supervisor
HTTP frontend -----/                              `-- task-worker
                                                       `-- one detached _task-exec per running task
```

`task-supervisor` is the launcher-facing process. It keeps one `task-worker` available and restarts it after failure. The worker schedules queued work but does not own running processes, and its liveness heartbeat runs independently from queue reconciliation so a slow store scan cannot falsely report it offline. Each `_task-exec` helper writes its own heartbeat, bounded logs, and terminal state directly to the store, so a worker or connector failure does not kill an already-started task.

After a machine reboot, queued tasks remain queued. A task that crossed the durable started marker but lost its executor heartbeat becomes `interrupted`; Scripthold never automatically repeats arbitrary potentially non-idempotent work. Submit an explicit new task when retry is appropriate.

## Public MCP tools

| Tool | Purpose |
|---|---|
| `task_run` | Validate, durably enqueue, and immediately return a task ID. |
| `task_list` | Page and filter bounded task metadata. |
| `task_get` | Read the latest state, terminal result, and bounded lifecycle history. |
| `task_logs` | Incrementally read stdout and stderr with independent absolute cursors. |
| `task_cancel` | Idempotently cancel queued work or terminate a running process tree. |

`task_run` requires `kind`, `idempotencyKey`, and the kind-specific fields. Duplicate names are allowed; the task ID is authoritative. Repeating an idempotency key with the identical canonical request returns the original task. Reusing it with different input fails without executing either request twice.

For `kind=script`, use `path`, optional `args`, and optional `cwd`. Admission hashes the regular script under the configured file-size limit; the worker revalidates the path and copies a SHA-256-matching snapshot into the private task directory before launch. Execution uses that private snapshot, closing the check-to-launch mutation window. For `kind=shell`, use `command`, optional `shell`, and optional `cwd`; only the working directory is confined, while the command remains unrestricted and runs as the server identity.

Optional `name`, `description`, and `tags` make the registry understandable. Optional `lockKeys` serialize tasks that share any key while unrelated tasks run in parallel up to `MCP_TASK_MAX_CONCURRENCY`.

`maxRuntimeSeconds` is optional. Zero or omission means unlimited unless the operator configures a nonzero global ceiling. Queue time never consumes runtime, and silence on stdout/stderr is not a timeout. Use `task_cancel` to stop unwanted work.

## States and recovery

The durable state machine is:

```text
queued -> starting -> running -> succeeded
                           |--> failed
                           |--> timed_out
                           |--> cancelled
                           `--> interrupted
```

Cancellation of `queued` work prevents launch. Cancellation of `running` work is polled by the executor and terminates the complete child process tree. Every state transition is serialized through the cross-process control lock before it becomes an immutable bounded lifecycle event containing only status, revision, timestamp, and an optional stable error code. A stale concurrent transition fails instead of creating two events with the same revision. Commands, arguments, paths, environment values, and output are excluded from lifecycle history.

The worker writes a started marker before process creation. Recovery may safely requeue a stale `starting` task only when that marker does not exist. Once the marker exists, recovery is at-most-once: loss of the executor produces `interrupted`, never an implicit rerun.

## Logs and retention

Each stream retains a fixed head and rolling tail. `task_logs` returns `cursor`, `nextCursor`, `availableEnd`, `droppedBytes`, and `truncated`; a caller whose cursor points into evicted middle output is advanced to the retained tail and told about the gap. Snapshot memory, disk bytes, and one response page are all bounded.

Retention runs only in the worker and never deletes `queued`, `starting`, or `running` tasks. Terminal tasks are removed oldest-first when age, terminal-count, or total-byte limits require it. Task state files, queue capacity, input arrays, text fields, list pages, state history, and directory scans have independent hard bounds.

## Configuration

| Variable | Default | Meaning |
|---|---:|---|
| `MCP_TASK_STORE_DIR` | unset | Enables the private owner-only task store. It must be absolute, link-free, outside public roots, and separate from the backup store. |
| `MCP_TASK_MAX_CONCURRENCY` | `2` | Maximum simultaneous starting/running tasks. |
| `MCP_TASK_MAX_QUEUED` | `64` | Maximum queued tasks. |
| `MCP_TASK_MAX_LOG_BYTES_PER_STREAM` | `8388608` | Retained head plus tail for each stdout/stderr stream. |
| `MCP_TASK_MAX_RUNTIME_SECONDS` | `0` | Global task runtime ceiling; zero means unlimited. |
| `MCP_TASK_RETENTION_DAYS` | `7` | Maximum ordinary age of terminal tasks. |
| `MCP_TASK_MAX_TERMINAL` | `1000` | Maximum retained terminal tasks. |
| `MCP_TASK_MAX_TOTAL_BYTES` | `536870912` | Total task-registry retention target. |

The descriptor permanently binds one store to its durability limits, but allowed directories are runtime configuration rather than persistent store identity. Operators may add or remove startup directories between restarts without recreating the task store. `task_run` validates and canonicalizes the working directory and script path at admission; the worker later revalidates those exact admitted paths as durable task authority rather than comparing them with the process's current root set. Script execution still requires the admitted size and SHA-256 digest to match before the worker creates its private snapshot. The descriptor and every store entry are owner-only; Windows uses a protected owner-only DACL and Unix uses owner/mode validation. Store roots, task commands, and task logs are never accessible through ordinary filesystem tools merely because a broader public root was configured.

Execution remains disabled by default. `kind=script` requires `MCP_ENABLE_RUN_SCRIPT=1` or `MCP_ENABLE_EXECUTION=1`; `kind=shell` requires `MCP_ENABLE_SHELL=1` or the combined flag. An HTTP frontend additionally requires `MCP_HTTP_ENABLE_EXECUTION=1`. The worker snapshots the same kind-specific gates, so an injected queue record cannot bypass them.

## Startup

Launch the supervisor before MCP frontends, using the same executable, task environment, and allowed directories:

```powershell
$env:MCP_TASK_STORE_DIR = "C:\Private\scripthold-tasks"
$env:MCP_ENABLE_RUN_SCRIPT = "1"
Start-Process .\scripthold_windows_amd64.exe -ArgumentList @("task-supervisor", "--", '"C:\Projects"') -WindowStyle Hidden
```

The four tracked PowerShell examples expose every task limit and start or reuse the supervisor. The task store is intentionally shared between the stdio and HTTP branches, while backup stores remain separate because each frontend process owns its backup-store writer lock.

## Compatibility

The synchronous public `run_script` and `shell` tools are not part of the 2.1.x catalog. Their authorization flags keep their meaning and authorize the corresponding `task_run` kind. Clients should submit work with `task_run`, poll `task_get` or `task_list`, page output with `task_logs`, and call `task_cancel` when necessary.
