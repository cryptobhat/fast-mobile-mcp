import pino from "pino";

export const logger = pino({
  level: process.env.LOG_LEVEL || "info",
  base: { service: "gateway-mcp" },
  timestamp: pino.stdTimeFunctions.isoTime
}, pino.destination({ fd: 2, sync: false }));