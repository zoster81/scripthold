& {
    # Native MCP Streamable HTTP quick start for Windows PowerShell 5.1.
    #
    # This example contains no credential. Create a private bearer-token file
    # before running it and keep execution tools disabled for the first test.

    Set-StrictMode -Version 3.0
    $ErrorActionPreference = "Stop"

    [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false)
    [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    $OutputEncoding = [Console]::OutputEncoding

    # --------------------------------------------------------------------------
    # Required configuration
    # --------------------------------------------------------------------------
    $McpServer = Join-Path $PSScriptRoot "scripthold_windows_amd64.exe"
    $AllowedDirectory = "C:\Path\To\AllowedProject"
    $TokenFile = "C:\Path\To\scripthold.token"
    $BackupStore = ""
    $TaskStore = "C:\Path\To\PrivateState\tasks"
    $TaskMaxConcurrency = 2
    $TaskMaxQueued = 64
    $TaskMaxLogBytesPerStream = 8388608
    $TaskMaxRuntimeSeconds = 0
    $TaskRetentionDays = 7
    $TaskMaxTerminal = 1000
    $TaskMaxTotalBytes = 536870912
    $LogDirectory = Join-Path $PSScriptRoot "logs"

    # --------------------------------------------------------------------------
    # Listener policy
    # --------------------------------------------------------------------------
    # Loopback HTTP is the safe default. A non-loopback listener requires
    # AllowNonLoopback plus TLS or an explicitly trusted private proxy boundary.
    $ListenAddress = "127.0.0.1:8765"
    $EndpointPath = "/mcp"
    $AllowNonLoopback = $false
    $AllowedHosts = ""
    $AllowedOrigins = ""
    $TrustedProxyCidrs = ""
    $TlsCertificateFile = ""
    $TlsKeyFile = ""

    # Keep execution disabled unless the client and deployment are fully trusted.
    # HTTP requires this flag plus the existing per-tool or combined flag.
    $EnableHttpExecution = $false
    $EnableRunScript = $false
    $EnableShell = $false

    function Assert-RegularFile {
        param(
            [Parameter(Mandatory = $true)]
            [string]$Path,

            [Parameter(Mandatory = $true)]
            [string]$Description
        )

        if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
            throw "$Description was not found: $Path"
        }
        $item = Get-Item -LiteralPath $Path -Force
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "$Description must not be a symbolic link or reparse point: $Path"
        }
    }

    function Set-BooleanEnvironmentFlag {
        param(
            [Parameter(Mandatory = $true)]
            [string]$Name,

            [Parameter(Mandatory = $true)]
            [bool]$Enabled
        )

        if ($Enabled) {
            [Environment]::SetEnvironmentVariable($Name, "1", "Process")
        } else {
            [Environment]::SetEnvironmentVariable($Name, $null, "Process")
        }
    }

    function Ensure-TaskSupervisor {
        if ([string]::IsNullOrWhiteSpace($TaskStore)) { return }
        $supervisorHeartbeat = Join-Path $TaskStore "supervisor.heartbeat"
        $supervisorFresh = $false
        if (Test-Path -LiteralPath $supervisorHeartbeat -PathType Leaf) {
            $age = [DateTime]::UtcNow - (Get-Item -LiteralPath $supervisorHeartbeat -Force).LastWriteTimeUtc
            $supervisorFresh = ($age.TotalSeconds -ge 0 -and $age.TotalSeconds -lt 5)
        }
        $supervisor = $null
        if (-not $supervisorFresh) {
            $stamp = Get-Date -Format "yyyyMMdd-HHmmss-fff"
            $supervisor = Start-Process -FilePath $McpServer -ArgumentList @("task-supervisor", "--", ('"{0}"' -f $AllowedDirectory)) -WindowStyle Hidden -RedirectStandardOutput (Join-Path $LogDirectory "task-supervisor-$stamp.stdout.log") -RedirectStandardError (Join-Path $LogDirectory "task-supervisor-$stamp.stderr.log") -PassThru
        }
        $heartbeat = Join-Path $TaskStore "worker.heartbeat"
        for ($attempt = 0; $attempt -lt 100; $attempt++) {
            if (Test-Path -LiteralPath $heartbeat -PathType Leaf) {
                $age = [DateTime]::UtcNow - (Get-Item -LiteralPath $heartbeat -Force).LastWriteTimeUtc
                if ($age.TotalSeconds -ge 0 -and $age.TotalSeconds -lt 5) { return }
            }
            if ($null -ne $supervisor -and $supervisor.HasExited) { throw "The durable task supervisor exited before worker readiness." }
            Start-Sleep -Milliseconds 100
        }
        throw "The durable task worker did not become ready."
    }

    if ($PSVersionTable.PSVersion.Major -lt 5) {
        throw "Windows PowerShell 5.1 or later is required."
    }

    Assert-RegularFile -Path $McpServer -Description "MCP server executable"
    Assert-RegularFile -Path $TokenFile -Description "HTTP bearer-token file"

    if (-not (Test-Path -LiteralPath $AllowedDirectory -PathType Container)) {
        throw "Allowed directory was not found: $AllowedDirectory"
    }
    $allowedItem = Get-Item -LiteralPath $AllowedDirectory -Force
    if (($allowedItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Allowed directory must not itself be a symbolic link or reparse point: $AllowedDirectory"
    }
    $AllowedDirectory = $allowedItem.FullName

    $token = [System.IO.File]::ReadAllText($TokenFile).Trim()
    if ($token.Length -lt 32 -or $token.IndexOf([char]0) -ge 0 -or $token -match '[\r\n]') {
        throw "The bearer-token file must contain one token of at least 32 characters."
    }
    $token = $null

    if (($TlsCertificateFile -eq "") -xor ($TlsKeyFile -eq "")) {
        throw "Configure both TLS certificate and key files, or neither."
    }
    if ($TlsCertificateFile -ne "") {
        Assert-RegularFile -Path $TlsCertificateFile -Description "TLS certificate"
        Assert-RegularFile -Path $TlsKeyFile -Description "TLS private key"
    }
    if (-not $AllowNonLoopback -and $ListenAddress -notmatch '^(127\.0\.0\.1|\[::1\]|localhost):\d+$') {
        throw "A non-loopback listener requires AllowNonLoopback = `$true."
    }
    if ($AllowNonLoopback -and
        $TlsCertificateFile -eq "" -and
        [string]::IsNullOrWhiteSpace($TrustedProxyCidrs)) {
        throw "A non-loopback listener requires TLS or a trusted proxy CIDR."
    }

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
        "MCP_HTTP_ALLOW_NON_LOOPBACK",
        "MCP_HTTP_ALLOWED_HOSTS",
        "MCP_HTTP_ALLOWED_ORIGINS",
        "MCP_HTTP_TRUSTED_PROXY_CIDRS",
        "MCP_HTTP_TLS_CERT_FILE",
        "MCP_HTTP_TLS_KEY_FILE",
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
    $previousEnvironment = @{}
    foreach ($name in $managedVariables) {
        $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
    }

    try {
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_API_KEY", $null, "Process")
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_TUNNEL_ID", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_SERVER_URL", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_COMMAND", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_EXTRA_HEADERS", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_DISCOVERY_EXTRA_HEADERS", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TUNNEL_AUTHORIZATION", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TRANSPORT", "streamable-http", "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_ADDR", $ListenAddress, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_PATH", $EndpointPath, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN_FILE", $TokenFile, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_ALLOWED_HOSTS", $AllowedHosts, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_ALLOWED_ORIGINS", $AllowedOrigins, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TRUSTED_PROXY_CIDRS", $TrustedProxyCidrs, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TLS_CERT_FILE", $TlsCertificateFile, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TLS_KEY_FILE", $TlsKeyFile, "Process")
        [Environment]::SetEnvironmentVariable("MCP_BACKUP_STORE_DIR", $BackupStore, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_STORE_DIR", $TaskStore, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_MAX_CONCURRENCY", $TaskMaxConcurrency.ToString(), "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_MAX_QUEUED", $TaskMaxQueued.ToString(), "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_MAX_LOG_BYTES_PER_STREAM", $TaskMaxLogBytesPerStream.ToString(), "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_MAX_RUNTIME_SECONDS", $TaskMaxRuntimeSeconds.ToString(), "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_RETENTION_DAYS", $TaskRetentionDays.ToString(), "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_MAX_TERMINAL", $TaskMaxTerminal.ToString(), "Process")
        [Environment]::SetEnvironmentVariable("MCP_TASK_MAX_TOTAL_BYTES", $TaskMaxTotalBytes.ToString(), "Process")
        [Environment]::SetEnvironmentVariable("MCP_STDIO_LEGACY_HANDSHAKE", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_ENABLE_EXECUTION", $null, "Process")
        Set-BooleanEnvironmentFlag -Name "MCP_HTTP_ALLOW_NON_LOOPBACK" -Enabled $AllowNonLoopback
        Set-BooleanEnvironmentFlag -Name "MCP_HTTP_ENABLE_EXECUTION" -Enabled $EnableHttpExecution
        Set-BooleanEnvironmentFlag -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
        Set-BooleanEnvironmentFlag -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell
        [void](New-Item -ItemType Directory -Path $LogDirectory -Force)
        Ensure-TaskSupervisor

        $scheme = if ($TlsCertificateFile -ne "") { "https" } else { "http" }
        Write-Host "Starting MCP Streamable HTTP at $scheme`://$ListenAddress$EndpointPath"
        Write-Host "Health: $scheme`://$ListenAddress/healthz"
        Write-Host "Readiness: $scheme`://$ListenAddress/readyz"
        & $McpServer --transport=streamable-http -- $AllowedDirectory
        if ($LASTEXITCODE -ne 0) {
            throw "MCP server exited with code $LASTEXITCODE."
        }
    } finally {
        foreach ($name in $managedVariables) {
            [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], "Process")
        }
    }
}
