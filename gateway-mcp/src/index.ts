import { loadConfig } from "./config.js";
import { logger } from "./logger.js";
import { startGateway } from "./mcp/server.js";

async function main() {
  const config = loadConfig(process.env);
  await startGateway(config);
}

main().catch((error) => {
  logger.error({ err: error }, "gateway crashed");
  process.exit(1);
});
