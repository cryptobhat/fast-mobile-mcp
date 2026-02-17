@echo off
setlocal
set "SCRIPT_DIR=%~dp0"
node "%SCRIPT_DIR%..\gateway-mcp\scripts\mcp-stdio.mjs"
exit /b %errorlevel%
