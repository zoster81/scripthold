& {
    # Reverse dual-transport example for Windows PowerShell 5.1.
    #
    # Topology:
    #   OpenAI tunnel-client -> authenticated loopback Streamable HTTP
    #   local MCP client     -> an independent foreground Scripthold stdio child
    #
    # When an MCP client launches this script, stdout is reserved for the local
    # stdio child. HTTP and tunnel logs are redirected to private local files.

    Set-StrictMode -Version 3.0
    $ErrorActionPreference = "Stop"

    [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false)
    [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    $OutputEncoding = [Console]::OutputEncoding

    $RuntimeApiKey = "REPLACE_WITH_RUNTIME_API_KEY"
    $TunnelId = "tunnel_REPLACE_WITH_ID"
    $AllowedDirectory = "C:\Path\To\AllowedProject"
    $TokenFile = "C:\Path\To\scripthold.token"
    $HttpBackupStore = "C:\Path\To\PrivateState\http"
    $StdioBackupStore = "C:\Path\To\PrivateState\stdio"
    $TaskStore = "C:\Path\To\PrivateState\tasks"
    $TaskMaxConcurrency = 2
    $TaskMaxQueued = 64
    $TaskMaxLogBytesPerStream = 8388608
    $TaskMaxRuntimeSeconds = 0
    $TaskRetentionDays = 7
    $TaskMaxTerminal = 1000
    $TaskMaxTotalBytes = 536870912

    $McpServerUrl = "http://127.0.0.1:8765/mcp"
    $McpListenAddress = "127.0.0.1:8765"
    $McpEndpointPath = "/mcp"
    $TunnelHealthBaseUrl = "http://127.0.0.1:8080"
    $LogDirectory = Join-Path $PSScriptRoot "logs"

    $EnableRunScript = $false
    $EnableShell = $false

    $TunnelClient = Join-Path $PSScriptRoot "tunnel-client.exe"
    $McpServer = Join-Path $PSScriptRoot "scripthold_windows_amd64.exe"

    $managedVariables = @(
        "CONTROL_PLANE_API_KEY",
        "CONTROL_PLANE_TUNNEL_ID",
        "MCP_SERVER_URL",
        "MCP_COMMAND",
        "MCP_EXTRA_HEADERS",
        "MCP_DISCOVERY_EXTRA_HEADERS",
        "MCP_TUNNEL_AUTHORIZATION",
        "MCP_TRANSPORT",
        "MCP_HTTP_ADDR",
        "MCP_HTTP_PATH",
        "MCP_HTTP_TOKEN",
        "MCP_HTTP_TOKEN_FILE",
        "MCP_HTTP_ENABLE_EXECUTION",
        "MCP_BACKUP_STORE_DIR",
        "MCP_TASK_STORE_DIR",
        "MCP_TASK_MAX_CONCURRENCY",
        "MCP_TASK_MAX_QUEUED",
        "MCP_TASK_MAX_LOG_BYTES_PER_STREAM",
        "MCP_TASK_MAX_RUNTIME_SECONDS",
        "MCP_TASK_RETENTION_DAYS",
        "MCP_TASK_MAX_TERMINAL",
        "MCP_TASK_MAX_TOTAL_BYTES",
        "MCP_STDIO_LEGACY_HANDSHAKE",
        "MCP_ENABLE_RUN_SCRIPT",
        "MCP_ENABLE_SHELL",
        "MCP_ENABLE_EXECUTION"
    )

    function Assert-RegularFile {
        param(
            [Parameter(Mandatory = $true)][string]$Path,
            [Parameter(Mandatory = $true)][string]$Description
        )

        if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
            throw "$Description was not found: $Path"
        }
        if (((Get-Item -LiteralPath $Path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "$Description must not be a symbolic link or reparse point: $Path"
        }
    }

    function Set-BooleanEnvironmentFlag {
        param(
            [Parameter(Mandatory = $true)][hashtable]$Values,
            [Parameter(Mandatory = $true)][string]$Name,
            [Parameter(Mandatory = $true)][bool]$Enabled
        )

        $Values[$Name] = if ($Enabled) { "1" } else { $null }
    }

    function Invoke-WithEnvironment {
        param(
            [Parameter(Mandatory = $true)][hashtable]$Values,
            [Parameter(Mandatory = $true)][scriptblock]$Action
        )

        $previous = @{}
        foreach ($name in $managedVariables) {
            $previous[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
            $value = if ($Values.ContainsKey($name)) { $Values[$name] } else { $null }
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
        try {
            & $Action
        } finally {
            foreach ($name in $managedVariables) {
                [Environment]::SetEnvironmentVariable($name, $previous[$name], "Process")
            }
        }
    }

    function Add-TaskEnvironment {
        param([Parameter(Mandatory = $true)][hashtable]$Values)
        $Values["MCP_TASK_STORE_DIR"] = $TaskStore
        $Values["MCP_TASK_MAX_CONCURRENCY"] = $TaskMaxConcurrency.ToString()
        $Values["MCP_TASK_MAX_QUEUED"] = $TaskMaxQueued.ToString()
        $Values["MCP_TASK_MAX_LOG_BYTES_PER_STREAM"] = $TaskMaxLogBytesPerStream.ToString()
        $Values["MCP_TASK_MAX_RUNTIME_SECONDS"] = $TaskMaxRuntimeSeconds.ToString()
        $Values["MCP_TASK_RETENTION_DAYS"] = $TaskRetentionDays.ToString()
        $Values["MCP_TASK_MAX_TERMINAL"] = $TaskMaxTerminal.ToString()
        $Values["MCP_TASK_MAX_TOTAL_BYTES"] = $TaskMaxTotalBytes.ToString()
    }

    function Ensure-TaskSupervisor {
        param([Parameter(Mandatory = $true)][hashtable]$Values)
        if ([string]::IsNullOrWhiteSpace($TaskStore)) { return }
        $supervisorHeartbeat = Join-Path $TaskStore "supervisor.heartbeat"
        $supervisorFresh = $false
        if (Test-Path -LiteralPath $supervisorHeartbeat -PathType Leaf) {
            $age = [DateTime]::UtcNow - (Get-Item -LiteralPath $supervisorHeartbeat -Force).LastWriteTimeUtc
            $supervisorFresh = ($age.TotalSeconds -ge 0 -and $age.TotalSeconds -lt 5)
        }
        $process = $null
        if (-not $supervisorFresh) {
            $process = Invoke-WithEnvironment -Values $Values -Action {
                Start-Process -FilePath $McpServer -ArgumentList @("task-supervisor", "--", ('"{0}"' -f $AllowedDirectory)) -WindowStyle Hidden -RedirectStandardOutput "NUL" -RedirectStandardError "NUL" -PassThru
            }
        }
        $workerHeartbeat = Join-Path $TaskStore "worker.heartbeat"
        for ($attempt = 0; $attempt -lt 100; $attempt++) {
            if (Test-Path -LiteralPath $workerHeartbeat -PathType Leaf) {
                $age = [DateTime]::UtcNow - (Get-Item -LiteralPath $workerHeartbeat -Force).LastWriteTimeUtc
                if ($age.TotalSeconds -ge 0 -and $age.TotalSeconds -lt 5) { return }
            }
            if ($null -ne $process -and $process.HasExited) { throw "The durable task supervisor exited before worker readiness." }
            Start-Sleep -Milliseconds 100
        }
        throw "The durable task worker did not become ready."
    }

    function Stop-OwnedProcess {
        param([System.Diagnostics.Process]$Process)

        if ($null -eq $Process) { return }
        try {
            if (-not $Process.HasExited) {
                $Process.Kill()
                [void]$Process.WaitForExit(5000)
            }
        } catch {}
    }

    function Wait-Ready {
        param(
            [Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process,
            [Parameter(Mandatory = $true)][string]$Url
        )

        for ($attempt = 0; $attempt -lt 80; $attempt++) {
            if ($Process.HasExited) { throw "A required background process exited during startup." }
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -Method Get -TimeoutSec 2
                if ($response.StatusCode -eq 200) { return }
            } catch {}
            Start-Sleep -Milliseconds 250
        }
        throw "Readiness timed out at $Url."
    }

    function Wait-TunnelProbe {
        param([Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process)

        for ($attempt = 0; $attempt -lt 120; $attempt++) {
            if ($Process.HasExited) { throw "tunnel-client exited during startup." }
            try {
                $status = Invoke-RestMethod -UseBasicParsing -Uri "$TunnelHealthBaseUrl/api/status" -Method Get -TimeoutSec 2
                $main = @($status.channels | Where-Object { $_.name -eq "main" })
                if ($main.Count -eq 1 -and $main[0].enabled -eq $true -and $main[0].probe_status -ceq "ok") { return }
            } catch {}
            Start-Sleep -Milliseconds 250
        }
        throw "The tunnel main channel did not reach probe_status=ok."
    }

    function Convert-ToProcessArgumentToken {
        param([Parameter(Mandatory = $true)][string]$Value)

        if ($Value -match '[\r\n"]') { throw "Process arguments must not contain quotes or line breaks." }
        $portable = $Value.Replace('\', '/')
        if ($portable -match '\s') { return '"' + $portable + '"' }
        return $portable
    }

    if ($PSVersionTable.PSVersion.Major -lt 5) { throw "Windows PowerShell 5.1 or later is required." }
    if ($RuntimeApiKey -eq "REPLACE_WITH_RUNTIME_API_KEY" -or [string]::IsNullOrWhiteSpace($RuntimeApiKey)) {
        throw "Replace the Runtime API key placeholder before running this script."
    }
    if ($TunnelId -eq "tunnel_REPLACE_WITH_ID" -or $TunnelId -notmatch '^tunnel_[0-9a-f]{32}$') {
        throw "Replace the Tunnel ID placeholder with tunnel_ followed by 32 lowercase hexadecimal characters."
    }
    Assert-RegularFile -Path $TunnelClient -Description "OpenAI tunnel client"
    Assert-RegularFile -Path $McpServer -Description "Scripthold server"
    Assert-RegularFile -Path $TokenFile -Description "HTTP bearer-token file"
    if (-not (Test-Path -LiteralPath $AllowedDirectory -PathType Container)) {
        throw "Set AllowedDirectory to an existing directory."
    }
    $allowedItem = Get-Item -LiteralPath $AllowedDirectory -Force
    if (($allowedItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "AllowedDirectory must not be a symbolic link or reparse point."
    }
    $AllowedDirectory = $allowedItem.FullName
    $McpServer = (Get-Item -LiteralPath $McpServer -Force).FullName
    $TunnelClient = (Get-Item -LiteralPath $TunnelClient -Force).FullName

    $token = [IO.File]::ReadAllText($TokenFile).Trim()
    if ($token.Length -lt 32 -or $token.IndexOf([char]0) -ge 0 -or $token -match '[\r\n]') {
        throw "The bearer-token file must contain one token of at least 32 characters."
    }
    if ([string]::Equals([IO.Path]::GetFullPath($StdioBackupStore), [IO.Path]::GetFullPath($HttpBackupStore), [StringComparison]::OrdinalIgnoreCase)) {
        throw "The stdio and HTTP processes require different backup stores."
    }
    if (-not (Test-Path -LiteralPath $LogDirectory -PathType Container)) {
        [void](New-Item -ItemType Directory -Path $LogDirectory)
    }
    $httpArguments = "--transport=streamable-http -- " + (Convert-ToProcessArgumentToken $AllowedDirectory)

    $httpEnvironment = @{
        "MCP_TRANSPORT" = "streamable-http"
        "MCP_HTTP_ADDR" = $McpListenAddress
        "MCP_HTTP_PATH" = $McpEndpointPath
        "MCP_HTTP_TOKEN_FILE" = $TokenFile
        "MCP_BACKUP_STORE_DIR" = $HttpBackupStore
    }
    Set-BooleanEnvironmentFlag -Values $httpEnvironment -Name "MCP_HTTP_ENABLE_EXECUTION" -Enabled ($EnableRunScript -or $EnableShell)
    Set-BooleanEnvironmentFlag -Values $httpEnvironment -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
    Set-BooleanEnvironmentFlag -Values $httpEnvironment -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell
    Add-TaskEnvironment -Values $httpEnvironment

    $tunnelEnvironment = @{
        "CONTROL_PLANE_API_KEY" = $RuntimeApiKey
        "CONTROL_PLANE_TUNNEL_ID" = $TunnelId
        "MCP_SERVER_URL" = $McpServerUrl
        "MCP_TUNNEL_AUTHORIZATION" = "Bearer $token"
        "MCP_EXTRA_HEADERS" = "Authorization: env:MCP_TUNNEL_AUTHORIZATION"
        "MCP_DISCOVERY_EXTRA_HEADERS" = "Authorization: env:MCP_TUNNEL_AUTHORIZATION"
    }
    $stdioEnvironment = @{
        "MCP_TRANSPORT" = "stdio"
        "MCP_BACKUP_STORE_DIR" = $StdioBackupStore
    }
    Set-BooleanEnvironmentFlag -Values $stdioEnvironment -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
    Set-BooleanEnvironmentFlag -Values $stdioEnvironment -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell
    Add-TaskEnvironment -Values $stdioEnvironment
    $taskEnvironment = @{}
    Set-BooleanEnvironmentFlag -Values $taskEnvironment -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
    Set-BooleanEnvironmentFlag -Values $taskEnvironment -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell
    Add-TaskEnvironment -Values $taskEnvironment

    $httpProcess = $null
    $tunnelProcess = $null
    try {
        Ensure-TaskSupervisor -Values $taskEnvironment
        $httpProcess = Invoke-WithEnvironment -Values $httpEnvironment -Action {
            Start-Process -FilePath $McpServer -ArgumentList $httpArguments -WindowStyle Hidden -RedirectStandardOutput (Join-Path $LogDirectory "http.stdout.log") -RedirectStandardError (Join-Path $LogDirectory "http.stderr.log") -PassThru
        }
        Wait-Ready -Process $httpProcess -Url ("http://" + $McpListenAddress + "/readyz")

        $doctor = Invoke-WithEnvironment -Values $tunnelEnvironment -Action {
            Start-Process -FilePath $TunnelClient -ArgumentList @("doctor", "--explain") -WindowStyle Hidden -RedirectStandardOutput (Join-Path $LogDirectory "doctor.stdout.log") -RedirectStandardError (Join-Path $LogDirectory "doctor.stderr.log") -Wait -PassThru
        }
        if ($doctor.ExitCode -ne 0) { throw "tunnel-client doctor failed with code $($doctor.ExitCode)." }

        $tunnelProcess = Invoke-WithEnvironment -Values $tunnelEnvironment -Action {
            Start-Process -FilePath $TunnelClient -ArgumentList @("run", "--health.listen-addr=127.0.0.1:8080", "--log.level=info", "--log.format=struct-text") -WindowStyle Hidden -RedirectStandardOutput (Join-Path $LogDirectory "tunnel.stdout.log") -RedirectStandardError (Join-Path $LogDirectory "tunnel.stderr.log") -PassThru
        }
        $tunnelEnvironment["MCP_TUNNEL_AUTHORIZATION"] = $null
        $RuntimeApiKey = $null
        $TunnelId = $null
        $token = $null
        Wait-Ready -Process $tunnelProcess -Url "$TunnelHealthBaseUrl/readyz"
        Wait-TunnelProbe -Process $tunnelProcess

        Invoke-WithEnvironment -Values $stdioEnvironment -Action {
            & $McpServer --transport=stdio -- $AllowedDirectory
            if ($LASTEXITCODE -ne 0) { throw "Local Scripthold stdio exited with code $LASTEXITCODE." }
        }
    } finally {
        Stop-OwnedProcess -Process $tunnelProcess
        Stop-OwnedProcess -Process $httpProcess
        $tunnelEnvironment["MCP_TUNNEL_AUTHORIZATION"] = $null
        $RuntimeApiKey = $null
        $TunnelId = $null
        $token = $null
        $httpArguments = $null
    }
}
