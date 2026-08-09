& {
    # Local stdio quick start for Windows PowerShell 5.1.
    #
    # Configure an MCP client to launch this script. Do not print status messages
    # to stdout: that stream belongs exclusively to MCP JSON-RPC.

    Set-StrictMode -Version 3.0
    $ErrorActionPreference = "Stop"

    [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false)
    [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    $OutputEncoding = [Console]::OutputEncoding

    $McpServer = Join-Path $PSScriptRoot "scripthold_windows_amd64.exe"
    $AllowedDirectory = "C:\Path\To\AllowedProject"
    $BackupStore = ""

    # Execution tools remain disabled unless explicitly enabled here.
    $EnableRunScript = $false
    $EnableShell = $false

    function Set-BooleanEnvironmentFlag {
        param(
            [Parameter(Mandatory = $true)][string]$Name,
            [Parameter(Mandatory = $true)][bool]$Enabled
        )

        if ($Enabled) {
            [Environment]::SetEnvironmentVariable($Name, "1", "Process")
        } else {
            [Environment]::SetEnvironmentVariable($Name, $null, "Process")
        }
    }

    if ($PSVersionTable.PSVersion.Major -lt 5) {
        throw "Windows PowerShell 5.1 or later is required."
    }
    if (-not (Test-Path -LiteralPath $McpServer -PathType Leaf)) {
        throw "Scripthold was not found: $McpServer"
    }
    if (-not (Test-Path -LiteralPath $AllowedDirectory -PathType Container)) {
        throw "Set AllowedDirectory to an existing directory."
    }
    $serverItem = Get-Item -LiteralPath $McpServer -Force
    $allowedItem = Get-Item -LiteralPath $AllowedDirectory -Force
    if (($serverItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
        ($allowedItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "The executable and allowed directory must not be reparse points."
    }
    $McpServer = $serverItem.FullName
    $AllowedDirectory = $allowedItem.FullName

    $managedVariables = @(
        "CONTROL_PLANE_API_KEY",
        "CONTROL_PLANE_TUNNEL_ID",
        "MCP_SERVER_URL",
        "MCP_COMMAND",
        "MCP_EXTRA_HEADERS",
        "MCP_DISCOVERY_EXTRA_HEADERS",
        "MCP_TUNNEL_AUTHORIZATION",
        "MCP_TRANSPORT",
        "MCP_HTTP_TOKEN",
        "MCP_HTTP_TOKEN_FILE",
        "MCP_HTTP_ENABLE_EXECUTION",
        "MCP_BACKUP_STORE_DIR",
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
        foreach ($name in @(
            "CONTROL_PLANE_API_KEY",
            "CONTROL_PLANE_TUNNEL_ID",
            "MCP_SERVER_URL",
            "MCP_COMMAND",
            "MCP_EXTRA_HEADERS",
            "MCP_DISCOVERY_EXTRA_HEADERS",
            "MCP_TUNNEL_AUTHORIZATION",
            "MCP_HTTP_TOKEN",
            "MCP_HTTP_TOKEN_FILE",
            "MCP_HTTP_ENABLE_EXECUTION",
            "MCP_STDIO_LEGACY_HANDSHAKE",
            "MCP_ENABLE_EXECUTION"
        )) {
            [Environment]::SetEnvironmentVariable($name, $null, "Process")
        }
        [Environment]::SetEnvironmentVariable("MCP_TRANSPORT", "stdio", "Process")
        [Environment]::SetEnvironmentVariable("MCP_BACKUP_STORE_DIR", $BackupStore, "Process")
        Set-BooleanEnvironmentFlag -Name "MCP_ENABLE_RUN_SCRIPT" -Enabled $EnableRunScript
        Set-BooleanEnvironmentFlag -Name "MCP_ENABLE_SHELL" -Enabled $EnableShell

        & $McpServer --transport=stdio -- $AllowedDirectory
        if ($LASTEXITCODE -ne 0) {
            throw "Scripthold exited with code $LASTEXITCODE."
        }
    } finally {
        foreach ($name in $managedVariables) {
            [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], "Process")
        }
    }
}
