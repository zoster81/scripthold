# Durable task execution

Use durable tasks for long-running local commands, such as builds or tests, that must continue after an MCP request ends. Submit work with `task_run`, then use `task_get` to check its state and `task_logs` to read output. The tool returns a `taskId` immediately; it does not wait for execution to finish.

Once a task is durably admitted, an MCP timeout, disconnect, or frontend restart does not by itself stop its executor. Tasks support explicit cancellation and recovery rules. **Script and shell execution remain disabled by default** and require the relevant server authorization settings.

This subsystem replaces the historical request-bound `run_script` and `shell` entry points. The sections below explain process ownership, limits, failure handling, and the exact execution contract.

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

`task_run` requires `kind`, `idempotencyKey`, and the kind-specific fields. Duplicate names are allowed; the task ID is authoritative. Repeating an idempotency key with the identical canonical request returns the original task. Reusing it with different input fails without executing either request twice. Admission waits for the shared control lock with the request context and checks cancellation again after lock acquisition; cancellation before durable admission therefore cannot create a task later merely because lock contention cleared.

`task_get.workerOnline` is intentionally scheduler-only: it reports the global worker heartbeat, not per-task executor health. It may therefore be `false` while a `starting` or `running` task executor is still healthy; clients must not infer task failure or resubmit work from that flag alone. A terminal state and `finishedAt` are authoritative for completion, and final-looking stdout/stderr is not process-exit evidence because log data can become visible before the terminal transition is persisted.

Execution behavior differs deliberately between a verified script and a shell command:

- **Script (`kind=script`):** use `path`, optional `args`, and optional `cwd`. Admission hashes the authorized regular script within its file-size limit. Before launch, the worker revalidates its path and copies the exact SHA-256-matching bytes into the private task directory. The executor runs **that verified snapshot**, closing the script check-to-launch modification window.
- **Shell (`kind=shell`):** use `command`, optional `shell`, and optional `cwd`. The working directory is confined, but the command itself is **unrestricted and executes as the server identity**. Windows accepts only the logical shell names `powershell`, `pwsh`, and `cmd`; Unix accepts `sh`, `bash`, and `pwsh`. Arbitrary executable paths or shell filenames are rejected as shell identifiers. Scripthold passes the command as a single shell argument without expanding PowerShell variables or interpolating its content.

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

Terminal errors distinguish failures that require different operator responses:

- **`TASK_START_FAILED`:** preparation or startup failed. Only a bounded, explicitly safe cause (such as an unavailable supported shell) may be shown; unclassified operating-system errors and private store paths are redacted.
- **`PROCESS_EXIT_NONZERO`:** the direct process exited with a nonzero code, which is reported. **`PROCESS_WAIT_FAILED`** is reserved for unclassified failures while waiting for the process.
- **`PROCESS_OUTPUT_DRAIN_TIMEOUT`:** the direct process exited but a descendant held inherited stdout/stderr handles open beyond the bounded `os/exec` drain period. Preserve the direct exit code when available; do not present incomplete output drainage as successful completion.

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

The task-store descriptor permanently binds its durability limits, **not** the process's current allowed-directory list. An operator may change startup roots across restarts without recreating the private store.

- **Admission and resumption:** `task_run` validates and canonicalizes `cwd` and any script path at admission. The worker revalidates those exact admitted paths as the task's durable authority, rather than treating subsequently changed process roots as a new admission policy. Scripts must still match their admitted size and SHA-256 before private-snapshot creation.
- **Private state:** the descriptor and every task entry remain owner-only (protected DACL on Windows; owner/mode validation on Unix). Public filesystem roots never make task-store directories, commands, or logs readable by ordinary file tools.

Execution remains disabled by default. `kind=script` requires `MCP_ENABLE_RUN_SCRIPT=1` or `MCP_ENABLE_EXECUTION=1`; `kind=shell` requires `MCP_ENABLE_SHELL=1` or the combined flag. An HTTP frontend additionally requires `MCP_HTTP_ENABLE_EXECUTION=1`. The worker snapshots the same kind-specific gates, so an injected queue record cannot bypass them.

## Startup

Launch the supervisor before MCP frontends, using the same executable, task environment, and allowed directories:

```powershell
$env:MCP_TASK_STORE_DIR = "C:\Private\scripthold-tasks"
$env:MCP_ENABLE_RUN_SCRIPT = "1"
Start-Process .\scripthold_windows_amd64.exe -ArgumentList @("task-supervisor", "--", '"C:\Projects"') -WindowStyle Hidden
```

The four tracked PowerShell examples expose every task limit and start or reuse the supervisor. The task store is intentionally shared between the stdio and HTTP branches, while backup stores remain separate because each frontend process owns its backup-store writer lock.

PowerShell shell tasks report the exit status of the PowerShell process itself. Windows PowerShell may run a native child that exits non-zero, continue with later successful PowerShell commands, and finally exit zero; Scripthold does not silently rewrite the caller's command to change that language/runtime behavior. Callers that require native-child failure propagation must express that policy explicitly in their PowerShell command or use an appropriate script, rather than relying on implicit command transformation.

## Compatibility

The synchronous public `run_script` and `shell` tools are historical and are not part of the current catalog. Their authorization flags retain their meaning for the corresponding `task_run` kind. Clients submit work with `task_run`, inspect it with `task_get` or `task_list`, page output with `task_logs`, and call `task_cancel` when necessary.
