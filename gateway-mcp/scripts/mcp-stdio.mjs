#!/usr/bin/env node
import { spawn, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..", "..");
const gatewayDir = path.join(repoRoot, "gateway-mcp");
const androidWorkerDir = path.join(repoRoot, "worker-android");
const iosWorkerDir = path.join(repoRoot, "worker-ios");
const isWindows = process.platform === "win32";

function log(message) {
  process.stderr.write(`[fast-mobile-mcp] ${message}\n`);
}

function parseBool(value, fallback) {
  if (value == null || value === "") return fallback;
  const normalized = value.trim().toLowerCase();
  if (["1", "true", "yes", "y", "on"].includes(normalized)) return true;
  if (["0", "false", "no", "n", "off"].includes(normalized)) return false;
  return fallback;
}

function appendToPath(envPath, entry) {
  if (!entry || !existsSync(entry)) return envPath;
  const parts = (envPath ?? "").split(path.delimiter).filter(Boolean);
  if (parts.includes(entry)) return envPath;
  return `${entry}${path.delimiter}${envPath ?? ""}`;
}

function runtimeEnv() {
  const env = { ...process.env };
  if (isWindows) {
    const userHome = env.USERPROFILE ?? "";
    const localAppData = env.LOCALAPPDATA ?? "";
    env.PATH = appendToPath(env.PATH, path.join(userHome, "tools", "go1.26.0", "bin"));
    env.PATH = appendToPath(env.PATH, path.join(userHome, "go", "bin"));
    env.PATH = appendToPath(env.PATH, path.join(localAppData, "Android", "Sdk", "platform-tools"));
    env.PATH = appendToPath(env.PATH, path.join(localAppData, "Android", "Sdk", "emulator"));
  }
  return env;
}

function npmCommand() {
  return isWindows ? "npm.cmd" : "npm";
}

function ensureGatewayReady(env) {
  const nodeModulesDir = path.join(gatewayDir, "node_modules");
  const distEntry = path.join(gatewayDir, "dist", "index.js");
  const allowBootstrap = parseBool(process.env.FMMCP_BOOTSTRAP, false);

  if (!allowBootstrap) {
    if (!existsSync(nodeModulesDir) || !existsSync(distEntry)) {
      throw new Error(
        "gateway is not prebuilt (missing node_modules or dist/index.js). " +
        "Run setup once: cd gateway-mcp && npm install && npm run build. " +
        "Set FMMCP_BOOTSTRAP=1 only if you explicitly want startup-time bootstrap."
      );
    }
    return distEntry;
  }

  if (!existsSync(nodeModulesDir)) {
    log("gateway dependencies missing, running npm install");
    const install = spawnSync(npmCommand(), ["install"], { cwd: gatewayDir, stdio: "inherit", env });
    if (install.status !== 0) {
      throw new Error("failed to install gateway dependencies");
    }
  }

  if (!existsSync(distEntry)) {
    log("gateway dist missing, running npm run build");
    const build = spawnSync(npmCommand(), ["run", "build"], { cwd: gatewayDir, stdio: "inherit", env });
    if (build.status !== 0) {
      throw new Error("failed to build gateway");
    }
  }

  if (!existsSync(distEntry)) {
    throw new Error("gateway dist/index.js is still missing after build");
  }
  return distEntry;
}

function prefixStream(stream, tag) {
  if (!stream) return;
  let pending = "";
  stream.on("data", (chunk) => {
    pending += chunk.toString();
    const lines = pending.split(/\r?\n/);
    pending = lines.pop() ?? "";
    for (const line of lines) {
      if (line.length > 0) process.stderr.write(`[${tag}] ${line}\n`);
    }
  });
  stream.on("end", () => {
    if (pending.length > 0) process.stderr.write(`[${tag}] ${pending}\n`);
  });
}

function workerCommand(workerDir) {
  const localBinary = path.join(workerDir, isWindows ? "worker.exe" : "worker");
  if (existsSync(localBinary)) {
    return { command: localBinary, args: [] };
  }

  const goBinary = isWindows
    ? path.join(process.env.USERPROFILE ?? "", "tools", "go1.26.0", "bin", "go.exe")
    : "go";

  if (isWindows && !existsSync(goBinary)) {
    return { command: "go", args: ["run", "./cmd/worker"] };
  }

  return { command: goBinary, args: ["run", "./cmd/worker"] };
}

function startWorker(label, workerDir, env) {
  const { command, args } = workerCommand(workerDir);
  log(`starting ${label} worker`);
  const child = spawn(command, args, {
    cwd: workerDir,
    env,
    stdio: ["ignore", "pipe", "pipe"]
  });

  child.on("error", (error) => {
    log(`${label} worker failed to start: ${error.message}`);
  });
  child.on("exit", (code, signal) => {
    log(`${label} worker exited (code=${code ?? "null"}, signal=${signal ?? "null"})`);
  });

  prefixStream(child.stdout, `${label}-worker`);
  prefixStream(child.stderr, `${label}-worker`);
  return child;
}

function terminateChild(child) {
  if (!child || child.killed || child.exitCode !== null) return;
  child.kill("SIGTERM");
  setTimeout(() => {
    if (!child.killed && child.exitCode === null) {
      child.kill("SIGKILL");
    }
  }, 1500).unref();
}

async function main() {
  const env = runtimeEnv();
  const gatewayEntry = ensureGatewayReady(env);

  const startAndroid = parseBool(process.env.FMMCP_START_ANDROID, true);
  const startIos = parseBool(process.env.FMMCP_START_IOS, process.platform === "darwin");

  const workerChildren = [];
  if (startAndroid) workerChildren.push(startWorker("android", androidWorkerDir, env));
  if (startIos) workerChildren.push(startWorker("ios", iosWorkerDir, env));
  if (!startAndroid && !startIos) {
    log("both local workers disabled; relying on externally running workers");
  }

  const gateway = spawn(process.execPath, [gatewayEntry], {
    cwd: gatewayDir,
    env,
    stdio: ["inherit", "inherit", "pipe"]
  });
  prefixStream(gateway.stderr, "gateway");

  let isShuttingDown = false;
  const shutdown = () => {
    if (isShuttingDown) return;
    isShuttingDown = true;
    terminateChild(gateway);
    for (const worker of workerChildren) terminateChild(worker);
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);

  gateway.on("exit", (code) => {
    for (const worker of workerChildren) terminateChild(worker);
    process.exit(code ?? 0);
  });
}

main().catch((error) => {
  log(`launcher failed: ${error.message}`);
  process.exit(1);
});
