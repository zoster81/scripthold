& {
    # OpenAI Secure MCP Tunnel quick start for Windows PowerShell 5.1.
    #
    # Topology:
    #   OpenAI tunnel-client -> one tunnel-owned Scripthold stdio child
    #   local clients        -> an independent loopback Streamable HTTP process
    #
    # Copy this file outside the repository before inserting credentials.

    Set-StrictMode -Version 3.0
    $ErrorActionPreference = "Stop"

    [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false)
    [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    $OutputEncoding = [Console]::OutputEncoding

    # --------------------------------------------------------------------------
    # Configuration
    # --------------------------------------------------------------------------
    $RuntimeApiKey = "REPLACE_WITH_RUNTIME_API_KEY"
    $TunnelId = "tunnel_REPLACE_WITH_ID"
    $AllowedDirectory = "C:\Path\To\AllowedProject"
    $TokenFile = "C:\Path\To\scripthold.token"
    $StdioBackupStore = "C:\Path\To\PrivateState\stdio"
    $HttpBackupStore = "C:\Path\To\PrivateState\http"

    $HttpListenAddress = "127.0.0.1:8765"
    $HttpEndpointPath = "/mcp"
    $TunnelHealthBaseUrl = "http://127.0.0.1:8080"

    $EnableRunScript = $false
    $EnableShell = $false

    # Place both executables next to this script, or change these paths.
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
        $item = Get-Item -LiteralPath $Path -Force
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
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

    function Stop-OwnedProcess {
        param([System.Diagnostics.Process]$Process)

        if ($null -eq $Process) { return }
        try {
            if (-not $Process.HasExited) {
                $Process.Kill()
                [void]$Process.WaitForExit(5000)
            }
        } catch {
            Write-Warning "Could not stop owned process $($Process.Id): $($_.Exception.Message)"
        }
    }

    function Wait-HttpReady {
        param(
            [Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process,
            [Parameter(Mandatory = $true)][string]$Url
        )

        for ($attempt = 0; $attempt -lt 80; $attempt++) {
            if ($Process.HasExited) { throw "Scripthold HTTP exited before becoming ready." }
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -Method Get -TimeoutSec 2
                if ($response.StatusCode -eq 200) { return }
            } catch {}
            Start-Sleep -Milliseconds 250
        }
        throw "Scripthold HTTP did not become ready at $Url."
    }

    function Wait-StdioChild {
        param([Parameter(Mandatory = $true)][System.Diagnostics.Process]$TunnelProcess)

        $expectedPath = [IO.Path]::GetFullPath($McpServer)
        $expectedName = [IO.Path]::GetFileName($expectedPath).Replace("'", "''")
        for ($attempt = 0; $attempt -lt 150; $attempt++) {
            if ($TunnelProcess.HasExited) {
                throw "tunnel-client exited before creating its stdio MCP child."
            }
            $children = @(Get-CimInstance Win32_Process -Filter ("Name = '{0}' AND ParentProcessId = {1}" -f $expectedName, $TunnelProcess.Id) -ErrorAction SilentlyContinue | Where-Object {
                $_.ExecutablePath -and
                [string]::Equals($_.ExecutablePath, $expectedPath, [StringComparison]::OrdinalIgnoreCase) -and
                $_.CommandLine -notmatch '(?i)--transport=streamable-http'
            })
            if ($children.Count -gt 1) {
                throw "tunnel-client created multiple stdio MCP children."
            }
            if ($children.Count -eq 1) {
                return Get-Process -Id ([int]$children[0].ProcessId) -ErrorAction Stop
            }
            Start-Sleep -Milliseconds 100
        }
        throw "Timed out waiting for the tunnel-owned stdio MCP child."
    }

    function Get-VerifiedTunnelStatus {
        $status = Invoke-RestMethod -UseBasicParsing -Uri "$TunnelHealthBaseUrl/api/status" -Method Get -TimeoutSec 2
        $mainChannels = @($status.channels | Where-Object { $_.name -eq "main" })
        if ($mainChannels.Count -ne 1) { return $false }
        return ($mainChannels[0].enabled -eq $true -and $mainChannels[0].probe_status -ceq "ok")
    }

    function Wait-TunnelReady {
        param([Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process)

        for ($attempt = 0; $attempt -lt 120; $attempt++) {
            if ($Process.HasExited) { throw "tunnel-client exited before becoming ready." }
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Uri "$TunnelHealthBaseUrl/readyz" -Method Get -TimeoutSec 2
                if ($response.StatusCode -eq 200 -and (Get-VerifiedTunnelStatus)) { return }
            } catch {}
            Start-Sleep -Milliseconds 250
        }
        throw "tunnel-client did not reach a ready main channel with probe_status=ok."
    }

    function Convert-ToMcpCommandToken {
        param([Parameter(Mandatory = $true)][string]$Value)

        if ($Value -match '[\r\n"]') { throw "MCP command paths must not contain quotes or line breaks." }
        $portable = $Value.Replace('\', '/')
        if ($portable -match '\s') { return '"' + $portable + '"' }
        return $portable
    }

    if ($PSVersionTable.PSVersion.Major -lt 5) {
        throw "Windows PowerShell 5.1 or later is required."
    }
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
    $token = $null
    if ([string]::Equals([IO.Path]::GetFullPath($StdioBackupStore), [IO.Path]::GetFullPath($HttpBackupStore), [StringComparison]::OrdinalIgnoreCase)) {
        throw "The stdio and HTTP processes require different backup stores."
    }

    $mcpCommand = @(
        (Convert-ToMcpCommandToken $McpServer),
        "--transport=stdio",
        "--",
        (Convert-ToMcpCommandToken $AllowedDirectory)
    ) -join " "
    $httpArguments = "--transport=streamable-http -- " + (Convert-ToMcpCommandToken $AllowedDirectory)

    $httpEnvironment = @{
        "MCP_TRANSPORT" = "streamable-http"
        "MCP_HTTP_ADDR" = $HttpListenAddress
        "MCP_HTTP_PATH" = $HttpEndpointPath
        "MCP_HTTP_TOKEN_FILE" = $TokenFile
        "MCP_BACKUP_STORE_DIR" = $HttpBackupStore
    }
    Set-BooleanEnvironmentFlag -Values $httpEnvironment -Name "MCP_HTTP_ENABLE_EXECUTION" -Enabled ($EnableRunScript -or $EnableShell)
    Set-BooleanEnvironmentFlag -Values $httpEnvironment -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
    Set-BooleanEnvironmentFlag -Values $httpEnvironment -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell

    $tunnelEnvironment = @{
        "CONTROL_PLANE_API_KEY" = $RuntimeApiKey
        "CONTROL_PLANE_TUNNEL_ID" = $TunnelId
        "MCP_COMMAND" = $mcpCommand
        "MCP_BACKUP_STORE_DIR" = $StdioBackupStore
        "MCP_STDIO_LEGACY_HANDSHAKE" = "1"
    }
    Set-BooleanEnvironmentFlag -Values $tunnelEnvironment -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
    Set-BooleanEnvironmentFlag -Values $tunnelEnvironment -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell

    $httpProcess = $null
    $tunnelProcess = $null
    $stdioProcess = $null
    $exitCode = 0
    try {
        $httpProcess = Invoke-WithEnvironment -Values $httpEnvironment -Action {
            Start-Process -FilePath $McpServer -ArgumentList $httpArguments -NoNewWindow -PassThru
        }
        Wait-HttpReady -Process $httpProcess -Url ("http://" + $HttpListenAddress + "/readyz")

        Write-Host "Checking the tunnel-to-stdio configuration..." -ForegroundColor Cyan
        $doctor = Invoke-WithEnvironment -Values $tunnelEnvironment -Action {
            Start-Process -FilePath $TunnelClient -ArgumentList @("doctor", "--explain") -NoNewWindow -Wait -PassThru
        }
        if ($doctor.ExitCode -ne 0) { throw "tunnel-client doctor failed with code $($doctor.ExitCode)." }

        $tunnelProcess = Invoke-WithEnvironment -Values $tunnelEnvironment -Action {
            Start-Process -FilePath $TunnelClient -ArgumentList @("run", "--health.listen-addr=127.0.0.1:8080", "--open-web-ui", "--log.level=info", "--log.format=struct-text") -NoNewWindow -PassThru
        }
        $RuntimeApiKey = $null
        $TunnelId = $null
        $stdioProcess = Wait-StdioChild -TunnelProcess $tunnelProcess
        Wait-TunnelReady -Process $tunnelProcess

        Write-Host "Ready: tunnel -> stdio PID $($stdioProcess.Id); local HTTP PID $($httpProcess.Id)." -ForegroundColor Green
        Write-Host "Local HTTP: http://$HttpListenAddress$HttpEndpointPath" -ForegroundColor DarkGray
        Write-Host "Tunnel UI: $TunnelHealthBaseUrl/ui" -ForegroundColor DarkGray
        Write-Host "Press Ctrl+C to stop both branches." -ForegroundColor DarkGray

        while (-not $tunnelProcess.WaitForExit(500)) {
            if ($stdioProcess.HasExited) { throw "The tunnel-owned stdio process exited unexpectedly." }
            if ($httpProcess.HasExited) { throw "The independent HTTP process exited unexpectedly." }
        }
        $exitCode = $tunnelProcess.ExitCode
        if ($exitCode -ne 0) { throw "tunnel-client stopped with exit code $exitCode." }
    } catch {
        $exitCode = 1
        Write-Error $_.Exception.Message -ErrorAction Continue
    } finally {
        Stop-OwnedProcess -Process $tunnelProcess
        Stop-OwnedProcess -Process $stdioProcess
        Stop-OwnedProcess -Process $httpProcess
        $RuntimeApiKey = $null
        $TunnelId = $null
        $mcpCommand = $null
        $httpArguments = $null
    }

    exit $exitCode
}
