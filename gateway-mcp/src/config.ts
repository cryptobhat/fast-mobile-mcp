import { z } from "zod";

const schema = z.object({
  MCP_GATEWAY_NAME: z.string().default("fast-mobile-mcp-gateway"),
  MCP_GATEWAY_VERSION: z.string().default("0.1.0"),
  ANDROID_WORKER_ADDR: z.string().default("127.0.0.1:50051"),
  IOS_WORKER_ADDR: z.string().default("127.0.0.1:50052"),
  GRPC_TIMEOUT_MS: z.coerce.number().int().positive().default(1200),
  GRPC_RETRIES: z.coerce.number().int().min(0).max(5).default(2),
  ACTION_QUEUE_IDLE_MS: z.coerce.number().int().positive().default(300000),
  MAX_UI_NODES: z.coerce.number().int().positive().default(300),
  MAX_ELEMENTS: z.coerce.number().int().positive().default(100),
  MAX_STREAM_FRAMES: z.coerce.number().int().positive().default(6),
  MAX_PAYLOAD_BYTES: z.coerce.number().int().positive().default(262144),
  LOG_LEVEL: z.enum(["fatal", "error", "warn", "info", "debug", "trace"]).default("info")
});

export type GatewayConfig = z.infer<typeof schema>;

export function loadConfig(env: NodeJS.ProcessEnv): GatewayConfig {
  return schema.parse(env);
}
