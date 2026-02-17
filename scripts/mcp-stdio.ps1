$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Launcher = Join-Path $ScriptDir "..\gateway-mcp\scripts\mcp-stdio.mjs"
node $Launcher
exit $LASTEXITCODE
