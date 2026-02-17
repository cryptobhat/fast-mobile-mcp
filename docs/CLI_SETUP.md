# MCP Client Setup

This project is a stdio MCP server. Different clients use different config formats, but all of them need the same command and args.

## Prerequisite Build Step

Run once before connecting any MCP client:

```powershell
cd C:\path\to\fast-mobile-mcp\gateway-mcp
npm install
npm run build
powershell -ExecutionPolicy Bypass -File C:\path\to\fast-mobile-mcp\scripts\gen-proto.ps1
```

## Recommended Server Command

Pick one launcher:

- Windows PowerShell:
  - command: `powershell`
  - args: `[-ExecutionPolicy, Bypass, -File, <repo>\\scripts\\mcp-stdio.ps1]`
- Windows cmd:
  - command: `<repo>\\scripts\\mcp-stdio.cmd`
  - args: `[]`
- Linux/macOS:
  - command: `<repo>/scripts/mcp-stdio.sh`
  - args: `[]`
- Cross-platform fallback:
  - command: `node`
  - args: `[<repo>/gateway-mcp/scripts/mcp-stdio.mjs]`

Replace `<repo>` with your local path.

## Codex CLI

```powershell
codex mcp add fast-mobile-mcp -- powershell -ExecutionPolicy Bypass -File C:\path\to\fast-mobile-mcp\scripts\mcp-stdio.ps1
codex mcp list
```

Alternative `~/.codex/config.toml` entry:

```toml
[mcp_servers.fast-mobile-mcp]
command = "powershell"
args = ["-ExecutionPolicy", "Bypass", "-File", "C:\\path\\to\\fast-mobile-mcp\\scripts\\mcp-stdio.ps1"]

[mcp_servers.fast-mobile-mcp.env]
FMMCP_START_ANDROID = "1"
FMMCP_START_IOS = "0"
```

## Claude Code

```powershell
claude mcp add-json fast-mobile-mcp "{\"type\":\"stdio\",\"command\":\"powershell\",\"args\":[\"-ExecutionPolicy\",\"Bypass\",\"-File\",\"C:\\\\path\\\\to\\\\fast-mobile-mcp\\\\scripts\\\\mcp-stdio.ps1\"],\"env\":{\"FMMCP_START_ANDROID\":\"1\",\"FMMCP_START_IOS\":\"0\"}}"
claude mcp list
```

## Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "fast-mobile-mcp": {
      "type": "stdio",
      "command": "powershell",
      "args": [
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        "C:\\path\\to\\fast-mobile-mcp\\scripts\\mcp-stdio.ps1"
      ],
      "env": {
        "FMMCP_START_ANDROID": "1",
        "FMMCP_START_IOS": "0"
      }
    }
  }
}
```

## Cursor

Add to project `.cursor/mcp.json` or global `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "fast-mobile-mcp": {
      "type": "stdio",
      "command": "powershell",
      "args": [
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        "C:\\path\\to\\fast-mobile-mcp\\scripts\\mcp-stdio.ps1"
      ],
      "env": {
        "FMMCP_START_ANDROID": "1",
        "FMMCP_START_IOS": "0"
      }
    }
  }
}
```

## Optional Environment Variables

- `FMMCP_START_ANDROID=0` disables local Android worker startup.
- `FMMCP_START_IOS=1` forces local iOS worker startup.

## Smoke Validation

After configuring your client, run this tool call first:

- `list_devices`

If needed, run runtime smoke from terminal:

```powershell
cd C:\path\to\fast-mobile-mcp\gateway-mcp
npm run e2e:smoke
```