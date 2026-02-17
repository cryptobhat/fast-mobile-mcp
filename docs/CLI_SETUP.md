# MCP Client Setup

This project is a stdio MCP server. Different clients use different config formats, but all of them need the same command and args.

## Recommended Server Command

Pick one launcher:

- Windows PowerShell:
  - command: `powershell`
  - args: `["-ExecutionPolicy","Bypass","-File","<repo>\\scripts\\mcp-stdio.ps1"]`
- Windows cmd:
  - command: `<repo>\\scripts\\mcp-stdio.cmd`
  - args: `[]`
- Linux/macOS:
  - command: `<repo>/scripts/mcp-stdio.sh`
  - args: `[]`
- Cross-platform fallback:
  - command: `node`
  - args: `["<repo>/gateway-mcp/scripts/mcp-stdio.mjs"]`

Replace `<repo>` with your local path.

## Optional Environment Variables

- `FMMCP_START_ANDROID=0` disables local Android worker startup.
- `FMMCP_START_IOS=1` forces local iOS worker startup.

## Generic MCP Config Shape

Use this pattern in your client config file:

```json
{
  "mcpServers": {
    "fast-mobile-mcp": {
      "command": "powershell",
      "args": [
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        "C:\\\\path\\\\to\\\\fast-mobile-mcp\\\\scripts\\\\mcp-stdio.ps1"
      ],
      "env": {
        "FMMCP_START_ANDROID": "1",
        "FMMCP_START_IOS": "0"
      }
    }
  }
}
```

If your client uses a different key name than `mcpServers`, map the same `command`, `args`, and `env` values into that format.
