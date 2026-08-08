& {
    # OpenAI Secure MCP Tunnel quick start for Windows PowerShell 5.1.
    #
    # Copy this file outside the repository before inserting real credentials.
    # Never commit a Runtime API key or a real Tunnel ID.

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

    # Keep execution disabled for the first test.
    # Set EnableRunScript to $true to allow supported scripts and executables whose
    # paths are inside an allowed directory. The child process is not sandboxed.
    # Set EnableShell to $true to allow unrestricted shell commands. Only the shell
    # working directory is validated; the command can access anything permitted to
    # the Windows identity running this process.
    # Review the execution-tool security section in TOOLS.md before enabling either.
    $EnableRunScript = $false
    $EnableShell = $false

    # Place both executables next to this script, or change these paths.
    $TunnelClient = Join-Path $PSScriptRoot "tunnel-client.exe"
    $McpServer = Join-Path $PSScriptRoot "scripthold_windows_amd64.exe"

    function Assert-FileExists {
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

    function Quote-McpToken {
        param(
            [Parameter(Mandatory = $true)]
            [string]$Value
        )

        if ($Value -match '\s') {
            return '"' + $Value.Replace('"', '\"') + '"'
        }

        return $Value
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

    if ($AllowedDirectory -eq "C:\Path\To\AllowedProject" -or
        -not (Test-Path -LiteralPath $AllowedDirectory -PathType Container)) {
        throw "Set AllowedDirectory to an existing directory that the MCP server may access."
    }

    $allowedItem = Get-Item -LiteralPath $AllowedDirectory -Force
    if (($allowedItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "AllowedDirectory must not be a symbolic link or reparse point: $AllowedDirectory"
    }
    $AllowedDirectory = $allowedItem.FullName

    Assert-FileExists -Path $TunnelClient -Description "OpenAI tunnel client"
    Assert-FileExists -Path $McpServer -Description "scripthold server"

    # The tunnel mcp.command parser treats backslashes as escape characters.
    # Forward slashes preserve Windows paths and are accepted by Windows APIs.
    $mcpCommandPath = $McpServer.Replace('\', '/')
    $allowedCommandPath = $AllowedDirectory.Replace('\', '/')
    $mcpCommand = "$(Quote-McpToken $mcpCommandPath) --transport=stdio $(Quote-McpToken $allowedCommandPath)"

    $managedVariables = @(
        "CONTROL_PLANE_API_KEY",
        "CONTROL_PLANE_TUNNEL_ID",
        "MCP_COMMAND",
        "MCP_STDIO_LEGACY_HANDSHAKE",
        "MCP_ENABLE_RUN_SCRIPT",
        "MCP_ENABLE_SHELL",
        "MCP_ENABLE_EXECUTION",
        "MCP_HTTP_TOKEN",
        "MCP_HTTP_TOKEN_FILE",
        "MCP_HTTP_ENABLE_EXECUTION"
    )
    $previousEnvironment = @{}
    foreach ($name in $managedVariables) {
        $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
    }

    $exitCode = 0

    try {
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_API_KEY", $RuntimeApiKey, "Process")
        [Environment]::SetEnvironmentVariable("CONTROL_PLANE_TUNNEL_ID", $TunnelId, "Process")
        [Environment]::SetEnvironmentVariable("MCP_COMMAND", $mcpCommand, "Process")
        [Environment]::SetEnvironmentVariable("MCP_STDIO_LEGACY_HANDSHAKE", "1", "Process")
        [Environment]::SetEnvironmentVariable("MCP_ENABLE_EXECUTION", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_TOKEN_FILE", $null, "Process")
        [Environment]::SetEnvironmentVariable("MCP_HTTP_ENABLE_EXECUTION", $null, "Process")
        [Environment]::SetEnvironmentVariable(
            "MCP_ENABLE_RUN_SCRIPT",
            $(if ($EnableRunScript) { "1" } else { $null }),
            "Process"
        )
        [Environment]::SetEnvironmentVariable(
            "MCP_ENABLE_SHELL",
            $(if ($EnableShell) { "1" } else { $null }),
            "Process"
        )
        $RuntimeApiKey = $null

        Write-Host "Checking tunnel configuration..." -ForegroundColor Cyan
        & $TunnelClient doctor --explain
        if ($LASTEXITCODE -ne 0) {
            throw "tunnel-client doctor failed with exit code $LASTEXITCODE."
        }

        Write-Host "Starting the OpenAI Secure MCP Tunnel..." -ForegroundColor Green
        Write-Host "Local operator UI: http://127.0.0.1:8080/ui" -ForegroundColor DarkGray
        Write-Host "Press Ctrl+C to stop." -ForegroundColor DarkGray

        & $TunnelClient run `
            "--health.listen-addr=127.0.0.1:8080" `
            "--open-web-ui" `
            "--log.level=info" `
            "--log.format=struct-text"

        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            throw "tunnel-client stopped with exit code $exitCode."
        }
    }
    catch {
        $exitCode = 1
        Write-Error $_.Exception.Message -ErrorAction Continue
    }
    finally {
        foreach ($name in $managedVariables) {
            [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], "Process")
        }
    }

    exit $exitCode
}
