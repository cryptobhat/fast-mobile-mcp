# fast-mobile-mcp

High-performance mobile automation architecture with a thin MCP gateway and dedicated Go workers for Android and iOS.

## Purpose

`fast-mobile-mcp` exists to make mobile automation usable from MCP clients (agents, CLIs, IDE assistants) with production-like behavior:

- one MCP endpoint for both Android and iOS
- strict per-device action serialization with cross-device parallelism
- snapshot-based element references (`ref_id`) for stable follow-up actions
- small response payloads for LLM-friendly tool usage
- binary screenshot streaming over gRPC between gateway and workers

Use this project if you want a mobile MCP server that is fast, deterministic, and easy to plug into any stdio-compatible MCP client.

## Monorepo Layout

- `gateway-mcp/`: MCP server (Node.js + TypeScript) with validation, per-device queueing, gRPC routing, retries, and response shaping.
- `worker-android/`: Android worker (Go) with cached discovery, persistent uiautomator2 clients, snapshot store, and serial per-device executors.
- `worker-ios/`: iOS worker (Go) with simulator discovery, persistent WebDriverAgent clients, snapshot store, and serial per-device executors.
- `proto/`: shared protobuf contract and generated code output location.
- `shared/`: shared config and shared Go packages (snapshot model).

## Prerequisites

- Go (1.22+)
- `protoc` (Protocol Buffers compiler)
- Node.js (20+) + npm
- `adb` in `PATH` for Android runtime
- `xcrun simctl` + WebDriverAgent for iOS runtime
- Android `uiautomator2` HTTP server reachable on forwarded device port (default `7912`)
- iOS WebDriverAgent reachable on configured host/port (default `http://127.0.0.1:8100+`)

## Quick Start

1. Install gateway dependencies:
   - `cd gateway-mcp && npm install`
2. Generate protobuf stubs:
   - Linux/macOS: `./scripts/gen-proto.sh`
   - Windows: `powershell -ExecutionPolicy Bypass -File .\scripts\gen-proto.ps1`
3. Build gateway:
   - `cd gateway-mcp && npm run build`

## One-Command MCP Server (for CLI clients)

Use this as your MCP server command in any stdio-compatible client:

- Windows PowerShell: `powershell -ExecutionPolicy Bypass -File <repo>\scripts\mcp-stdio.ps1`
- Windows cmd: `<repo>\scripts\mcp-stdio.cmd`
- Linux/macOS: `<repo>/scripts/mcp-stdio.sh`
- Fallback: `node <repo>/gateway-mcp/scripts/mcp-stdio.mjs`

Important: replace `<repo>` with your real local path. Do not paste `C:\path\to\...` literally.

Detailed client config mapping: `docs/CLI_SETUP.md`
Includes explicit setup for Codex CLI, Claude Code, Claude Desktop, and Cursor.

What this launcher does:

- starts local Android worker by default
- starts local iOS worker only on macOS by default
- starts gateway on stdio so MCP clients can connect directly

Launcher env switches:

- `FMMCP_START_ANDROID=0` to disable local Android worker
- `FMMCP_START_IOS=1` to force starting local iOS worker
- `FMMCP_BOOTSTRAP=1` to allow startup-time `npm install/build` (disabled by default for faster, stable MCP handshakes)

## Install in Popular MCP Clients

Use this repo's launcher command in your client config:

- `powershell -ExecutionPolicy Bypass -File C:\path\to\fast-mobile-mcp\scripts\mcp-stdio.ps1`

### Codex CLI

```powershell
codex mcp add fast-mobile-mcp -- powershell -ExecutionPolicy Bypass -File C:\path\to\fast-mobile-mcp\scripts\mcp-stdio.ps1
codex mcp list
```

`~/.codex/config.toml` alternative:

```toml
[mcp_servers.fast-mobile-mcp]
command = "powershell"
args = ["-ExecutionPolicy", "Bypass", "-File", "C:\\path\\to\\fast-mobile-mcp\\scripts\\mcp-stdio.ps1"]

[mcp_servers.fast-mobile-mcp.env]
FMMCP_START_ANDROID = "1"
FMMCP_START_IOS = "0"
```

### Claude Code

```powershell
claude mcp add-json fast-mobile-mcp "{\"type\":\"stdio\",\"command\":\"powershell\",\"args\":[\"-ExecutionPolicy\",\"Bypass\",\"-File\",\"C:\\\\path\\\\to\\\\fast-mobile-mcp\\\\scripts\\\\mcp-stdio.ps1\"],\"env\":{\"FMMCP_START_ANDROID\":\"1\",\"FMMCP_START_IOS\":\"0\"}}"
claude mcp list
```

### Claude Desktop

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

### Startup Error Fix (Codex)

If Codex shows:

- `MCP startup failed: ... initialize response`

it is usually a bad script path in the MCP config. Verify and fix with:

```powershell
codex mcp get fast-mobile-mcp
codex mcp remove fast-mobile-mcp
codex mcp add fast-mobile-mcp -- powershell -ExecutionPolicy Bypass -File C:\Users\nagar\fast-mobile-mcp\scripts\mcp-stdio.ps1
codex mcp list
```

### Cursor

Add to `.cursor/mcp.json` or `~/.cursor/mcp.json`:

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

## MCP Tool Surface

The gateway exposes these tools:

- `list_devices`
- `get_active_app`
- `get_ui_tree`
- `find_elements`
- `tap`
- `type`
- `swipe`
- `screenshot_stream`

## Runtime Smoke E2E

Run a runtime smoke check through MCP (workers + gateway):

- `cd gateway-mcp && npm run e2e:smoke`

What it validates:

- tool registration (`tools/list`)
- `list_devices` returns at least one ready device
- `get_ui_tree` returns a snapshot
- `find_elements` works against that snapshot
- `screenshot_stream` returns frame metadata
- invalid device request fails as expected

Note: action-level checks (`get_ui_tree`, tap/type/swipe, screenshots) require platform automation servers to be alive:

- Android: `uiautomator2` endpoint responds to `/version`
- iOS: WDA endpoint responds to `/status`

Optional E2E env switches:

- `FMMCP_E2E_DEVICE_ID=<device-id>` to target a specific device
- `FMMCP_E2E_PLATFORM=PLATFORM_ANDROID|PLATFORM_IOS` to target platform
- `FMMCP_E2E_ENABLE_ACTIONS=1` to include a small swipe action check

## Quick Verification for Reviewers

For anyone checking the repo directly, this is the fastest verification path:

1. `cd gateway-mcp && npm install && npm run build`
2. from repo root: `powershell -ExecutionPolicy Bypass -File .\scripts\gen-proto.ps1` (Windows) or `./scripts/gen-proto.sh` (Linux/macOS)
3. `cd worker-android && go build ./cmd/worker`
4. `cd worker-ios && go build ./cmd/worker`
5. `cd gateway-mcp && npm run e2e:smoke` (requires runtime dependencies and at least one ready device)

## Build Targets

- `make proto`
- `make build-gateway`
- `make build-workers`
- `make mcp-stdio`
- `make e2e-smoke`

## Runtime Design Guarantees

- parallel automation across devices
- strict serial execution per device
- snapshot-based `ref_id` addressing
- small, shaped payloads to MCP clients
- gRPC binary screenshot chunk streaming

## Environment

- Gateway env: `gateway-mcp/.env.example`
- Android env: `worker-android/.env.example`
- iOS env: `worker-ios/.env.example`
- Unified sample config: `shared/config/sample.yaml`
