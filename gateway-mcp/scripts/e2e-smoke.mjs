#!/usr/bin/env node
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const launcherPath = path.join(scriptDir, "mcp-stdio.mjs");
const gatewayCwd = path.resolve(scriptDir, "..");

const expectedTools = [
  "list_devices",
  "get_active_app",
  "get_ui_tree",
  "find_elements",
  "tap",
  "type",
  "swipe",
  "screenshot_stream"
];

function parseBool(value, fallback) {
  if (value == null || value === "") return fallback;
  const normalized = value.trim().toLowerCase();
  if (["1", "true", "yes", "y", "on"].includes(normalized)) return true;
  if (["0", "false", "no", "n", "off"].includes(normalized)) return false;
  return fallback;
}

function parseToolJson(name, result) {
  if (result?.isError) {
    throw new Error(`${name} returned isError=true`);
  }

  const textContent = result?.content?.find((item) => item.type === "text")?.text;
  if (!textContent) {
    throw new Error(`${name} did not return text content`);
  }

  try {
    return JSON.parse(textContent);
  } catch (error) {
    throw new Error(`${name} returned non-JSON text: ${String(error)}`);
  }
}

async function callToolJson(client, name, args) {
  const result = await client.callTool({
    name,
    arguments: args ?? {}
  });
  return parseToolJson(name, result);
}

function chooseDevice(devices) {
  const wantedDeviceId = process.env.FMMCP_E2E_DEVICE_ID;
  if (wantedDeviceId) {
    const match = devices.find((device) => device.device_id === wantedDeviceId);
    if (!match) throw new Error(`requested device id not found: ${wantedDeviceId}`);
    return match;
  }

  const wantedPlatform = process.env.FMMCP_E2E_PLATFORM;
  if (wantedPlatform) {
    const match = devices.find((device) => device.platform === wantedPlatform);
    if (!match) throw new Error(`requested platform not found: ${wantedPlatform}`);
    return match;
  }

  return devices[0];
}

async function expectInvalidDeviceFailure(client) {
  const invalidDeviceId = "__fast_mobile_mcp_invalid_device__";
  try {
    const result = await client.callTool({
      name: "get_ui_tree",
      arguments: {
        device_id: invalidDeviceId,
        node_limit: 1
      }
    });

    if (result?.isError) return;
    throw new Error("invalid device unexpectedly succeeded");
  } catch {
    return;
  }
}

async function maybeRunActionCheck(client, deviceId) {
  if (!parseBool(process.env.FMMCP_E2E_ENABLE_ACTIONS, false)) return;

  await callToolJson(client, "swipe", {
    device_id: deviceId,
    direction: "DIRECTION_UP",
    distance_px: 40,
    duration_ms: 120
  });
}

async function main() {
  const transport = new StdioClientTransport({
    command: process.execPath,
    args: [launcherPath],
    cwd: gatewayCwd,
    env: { ...process.env },
    stderr: "inherit"
  });

  const client = new Client(
    { name: "fast-mobile-mcp-e2e-smoke", version: "0.1.0" },
    { capabilities: {} }
  );

  await client.connect(transport);
  try {
    const toolResult = await client.listTools();
    const available = new Set(toolResult.tools.map((tool) => tool.name));
    for (const required of expectedTools) {
      if (!available.has(required)) {
        throw new Error(`missing expected tool: ${required}`);
      }
    }

    const devicePayload = await callToolJson(client, "list_devices", { ready_only: true });
    const devices = Array.isArray(devicePayload.devices) ? devicePayload.devices : [];
    if (devices.length === 0) {
      throw new Error("no ready devices found; connect a device/simulator and ensure adb/xcrun + WDA are healthy");
    }

    const targetDevice = chooseDevice(devices);
    if (!targetDevice?.device_id) {
      throw new Error("selected device is missing device_id");
    }

    const tree = await callToolJson(client, "get_ui_tree", {
      device_id: targetDevice.device_id,
      force_refresh: true,
      node_limit: 120
    });
    if (!tree.snapshot_id) {
      throw new Error("get_ui_tree did not return snapshot_id");
    }
    if (!Array.isArray(tree.nodes)) {
      throw new Error("get_ui_tree did not return nodes");
    }

    const elements = await callToolJson(client, "find_elements", {
      device_id: targetDevice.device_id,
      snapshot_id: tree.snapshot_id,
      limit: 5,
      include_nodes: true
    });
    if (!Array.isArray(elements.elements)) {
      throw new Error("find_elements did not return elements");
    }

    const frames = await callToolJson(client, "screenshot_stream", {
      device_id: targetDevice.device_id,
      max_frames: 1,
      max_fps: 1
    });
    const frameCount = Number(frames.frame_count ?? 0);
    if (!Number.isFinite(frameCount) || frameCount < 1) {
      throw new Error("screenshot_stream did not return any frame metadata");
    }

    await maybeRunActionCheck(client, targetDevice.device_id);
    await expectInvalidDeviceFailure(client);

    process.stdout.write(`E2E smoke passed for ${targetDevice.device_id} (${targetDevice.platform})\n`);
  } finally {
    await transport.close().catch(() => {});
  }
}

main().catch((error) => {
  process.stderr.write(`E2E smoke failed: ${error.message}\n`);
  process.exit(1);
});
