& {
    # OpenAI Secure MCP Tunnel quick start for Windows PowerShell 5.1.
    #
    # This launcher keeps Scripthold on authenticated loopback Streamable HTTP
    # and points tunnel-client at that endpoint. Copy this file outside the
    # repository before inserting real control-plane credentials.

    Set-StrictMode -Version 3.0
    $ErrorActionPreference = "Stop"

    [Console]::InputEncoding = [System.Text.UTF8Encoding]::new($false)
    [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
    $OutputEncoding = [Console]::OutputEncoding

    # --------------------------------------------------------------------------
    # Configuration
    # --------------------------------------------------------------------------
    $RuntimeApiKey = "REPLACE_WITH_RUNTIME_API_KEY"
    $TunnelId = "tunnel_REPLACE_WITH_ID"
    $AllowedDirectory = "C:\Path\To\AllowedProject"
    $TokenFile = "C:\Path\To\scripthold.token"

    $McpServerUrl = "http://127.0.0.1:8765/mcp"
    $McpListenAddress = "127.0.0.1:8765"
    $McpEndpointPath = "/mcp"
    $TunnelHealthBaseUrl = "http://127.0.0.1:8080"

    # Keep execution disabled for the first test. HTTP execution requires the
    # transport gate plus the selected per-tool gate.
    $EnableRunScript = $false
    $EnableShell = $false

    # Place both executables next to this script, or change these paths.
    $TunnelClient = Join-Path $PSScriptRoot "tunnel-client.exe"
    $McpServer = Join-Path $PSScriptRoot "scripthold_windows_amd64.exe"

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

    function Stop-OwnedProcess {
        param(
            [System.Diagnostics.Process]$Process
        )

        if ($null -eq $Process) {
            return
        }
        try {
            if (-not $Process.HasExited) {
                $Process.Kill()
                [void]$Process.WaitForExit(5000)
            }
        } catch {
            Write-Warning "Could not stop owned process cleanly: $($_.Exception.Message)"
        }
    }

    function Wait-McpReady {
        param(
            [Parameter(Mandatory = $true)]
            [System.Diagnostics.Process]$Process
        )

        $readyUrl = "http://$McpListenAddress/readyz"
        for ($attempt = 0; $attempt -lt 80; $attempt++) {
            if ($Process.HasExited) {
                throw "Scripthold exited before becoming ready."
            }
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Uri $readyUrl -Method Get -TimeoutSec 2
                if ($response.StatusCode -eq 200) {
                    return
                }
            } catch {
                # Startup races are expected until the listener is ready.
            }
            Start-Sleep -Milliseconds 250
        }
        throw "Scripthold did not become ready at $readyUrl."
    }

    function Get-VerifiedTunnelStatus {
        $statusUrl = "$TunnelHealthBaseUrl/api/status"
        $status = Invoke-RestMethod -UseBasicParsing -Uri $statusUrl -Method Get -TimeoutSec 2
        $mainChannels = @($status.channels | Where-Object { $_.name -eq "main" })
        if ($mainChannels.Count -ne 1) {
            return $false
        }
        $main = $mainChannels[0]
        return ($main.enabled -eq $true -and $main.probe_status -eq "ok")
    }

    function Wait-TunnelReady {
        param(
            [Parameter(Mandatory = $true)]
            [System.Diagnostics.Process]$Process
        )

        $readyUrl = "$TunnelHealthBaseUrl/readyz"
        for ($attempt = 0; $attempt -lt 120; $attempt++) {
            if ($Process.HasExited) {
                throw "tunnel-client exited before becoming ready."
            }
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Uri $readyUrl -Method Get -TimeoutSec 2
                if ($response.StatusCode -eq 200 -and (Get-VerifiedTunnelStatus)) {
                    return
                }
            } catch {
                # Readiness and MCP probe state may lag process startup.
            }
            Start-Sleep -Milliseconds 250
        }
        throw "tunnel-client did not reach a ready main channel with probe_status=ok."
    }

    if ($PSVersionTable.PSVersion.Major -lt 5) {
        throw "Windows PowerShell 5.1 or later is required."
    }

    if ($RuntimeApiKey -eq "REPLACE_WITH_RUNTIME_API_KEY" -or
        [string]::IsNullOrWhiteSpace($RuntimeApiKey)) {
        throw "Replace the Runtime API key placeholder before running this script."
    }
    if ($TunnelId -eq "tunnel_REPLACE_WITH_ID" -or
        $TunnelId -notmatch '^tunnel_[0-9a-f]{32}$') {
        throw "Replace the Tunnel ID placeholder with tunnel_ followed by 32 lowercase hexadecimal characters."
    }
    if (-not (Test-Path -LiteralPath $AllowedDirectory -PathType Container)) {
        throw "Set AllowedDirectory to an existing directory that the MCP server may access."
    }

    $allowedItem = Get-Item -LiteralPath $AllowedDirectory -Force
    if (($allowedItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "AllowedDirectory must not be a symbolic link or reparse point: $AllowedDirectory"
    }
    $AllowedDirectory = $allowedItem.FullName

    Assert-RegularFile -Path $TunnelClient -Description "OpenAI tunnel client"
    Assert-RegularFile -Path $McpServer -Description "Scripthold server"
    Assert-RegularFile -Path $TokenFile -Description "HTTP bearer-token file"

    $token = [System.IO.File]::ReadAllText($TokenFile).Trim()
    if ($token.Length -lt 32 -or $token.IndexOf([char]0) -ge 0 -or $token -match '[\r\n]') {
        throw "The bearer-token file must contain one token of at least 32 characters."
    }

    $managedVariables = @(
        "CONTROL_PLANE_API_KEY",
        "CONTROL_PLANE_TUNNEL_ID",
        "MCP_SERVER_URL",
        "MCP_EXTRA_HEADERS",
        "MCP_DISCOVERY_EXTRA_HEADERS",
        "MCP_TUNNEL_AUTHORIZATION",
        "MCP_TRANSPORT",
        "MCP_HTTP_ADDR",
        "MCP_HTTP_PATH",
        "MCP_HTTP_TOKEN",
        "MCP_HTTP_TOKEN_FILE",
        "MCP_HTTP_ENABLE_EXECUTION",
        "MCP_ENABLE_RUN_SCRIPT",
        "MCP_ENABLE_SHELL",
        "MCP_ENABLE_EXECUTION"
    )
    $previousEnvironment = @{}
    foreach ($name in $managedVariables) {
        $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
    }

    $mcpProcess = $null
    $tunnelProcess = $null
    $exitCode = 0

    try {
        # Start Scripthold first. The bearer token stays in its private file and
        # is removed from the child environment by Scripthold after config load.
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_API_KEY", $null, "Process")
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_TUNNEL_ID", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_SERVER_URL", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_EXTRA_HEADERS", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_DISCOVERY_EXTRA_HEADERS", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TUNNEL_AUTHORIZATION", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TRANSPORT", "streamable-http", "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_ADDR", $McpListenAddress, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_PATH", $McpEndpointPath, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN_FILE", $TokenFile, "Process")
        [Environment]::SetEnvironmentVariable("MCP_ENABLE_EXECUTION", $null, "Process")
        Set-BooleanEnvironmentFlag -Name "MCP_HTTP_ENABLE_EXECUTION" -Enabled ($EnableRunScript -or $EnableShell)
        Set-BooleanEnvironmentFlag -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
        Set-BooleanEnvironmentFlag -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell

        $serverArguments = "--transport=streamable-http -- `"$AllowedDirectory`""
        $mcpProcess = Start-Process -FilePath $McpServer -ArgumentList $serverArguments -NoNewWindow -PassThru
        Wait-McpReady -Process $mcpProcess

        # Configure tunnel-client to use authenticated Streamable HTTP. Header
        # values are referenced from environment rather than embedded in argv.
        [Environment]::SetEnvironmentVariable("MCP_TRANSPORT", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_ADDR", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_PATH", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN_FILE", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_ENABLE_EXECUTION", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_ENABLE_RUN_SCRIPT", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_ENABLE_SHELL", $null, "Process")
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_API_KEY", $RuntimeApiKey, "Process")
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_TUNNEL_ID", $TunnelId, "Process")
        [Environment]::SetEnvironmentVariable("MCP_SERVER_URL", $McpServerUrl, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TUNNEL_AUTHORIZATION", "Bearer $token", "Process")
        [Environment]::SetEnvironmentVariable("MCP_EXTRA_HEADERS", "Authorization: env:MCP_TUNNEL_AUTHORIZATION", "Process")
        [Environment]::SetEnvironmentVariable("MCP_DISCOVERY_EXTRA_HEADERS", "Authorization: env:MCP_TUNNEL_AUTHORIZATION", "Process")
        $RuntimeApiKey = $null
        $TunnelId = $null
        $token = $null

        Write-Host "Checking tunnel configuration..." -ForegroundColor Cyan
        & $TunnelClient doctor --explain
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "tunnel-client doctor reported exit code $LASTEXITCODE; runtime readiness and the main MCP probe will be checked before success is reported."
        }

        Write-Host "Starting the OpenAI Secure MCP Tunnel..." -ForegroundColor Green
        $tunnelArguments = @(
            "run",
            "--health.listen-addr=127.0.0.1:8080",
            "--open-web-ui",
            "--log.level=info",
            "--log.format=struct-text"
        )
        $tunnelProcess = Start-Process -FilePath $TunnelClient -ArgumentList $tunnelArguments -NoNewWindow -PassThru
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_API_KEY", $null, "Process")
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_TUNNEL_ID", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_TUNNEL_AUTHORIZATION", $null, "Process")
        Wait-TunnelReady -Process $tunnelProcess

        Write-Host "Tunnel ready: main channel enabled with probe_status=ok." -ForegroundColor Green
        Write-Host "Local operator UI: $TunnelHealthBaseUrl/ui" -ForegroundColor DarkGray
        Write-Host "Press Ctrl+C to stop." -ForegroundColor DarkGray

        $tunnelProcess.WaitForExit()
        $exitCode = $tunnelProcess.ExitCode
        if ($exitCode -ne 0) {
            throw "tunnel-client stopped with exit code $exitCode."
        }
    }
    catch {
        $exitCode = 1
        Write-Error $_.Exception.Message -ErrorAction Continue
    }
    finally {
        Stop-OwnedProcess -Process $tunnelProcess
        Stop-OwnedProcess -Process $mcpProcess
        foreach ($name in $managedVariables) {
            [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], "Process")
        }
        $token = $null
        $RuntimeApiKey = $null
        $TunnelId = $null
    }

    exit $exitCode
}
