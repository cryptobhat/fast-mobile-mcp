import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { CallToolRequestSchema, ListToolsRequestSchema } from "@modelcontextprotocol/sdk/types.js";
import { GatewayConfig } from "../config.js";
import { MobileGrpcRouter } from "../grpc/mobileClient.js";
import { logger } from "../logger.js";
import { DeviceQueueManager } from "../queue/deviceQueue.js";
import { shapeAction, shapeDevices, shapeElements, shapeScreenshotEvents, shapeTree } from "../response/shaper.js";
import {
  activeAppSchema,
  findElementsSchema,
  listDevicesSchema,
  screenshotStreamSchema,
  swipeSchema,
  tapSchema,
  typeSchema,
  uiTreeSchema
} from "../validation.js";

function asMcpText(result: unknown) {
  return {
    content: [{ type: "text", text: JSON.stringify(result) }]
  };
}

export async function startGateway(config: GatewayConfig): Promise<void> {
  const grpc = new MobileGrpcRouter({
    androidAddr: config.ANDROID_WORKER_ADDR,
    iosAddr: config.IOS_WORKER_ADDR,
    timeoutMs: config.GRPC_TIMEOUT_MS,
    retries: config.GRPC_RETRIES
  });

  const queue = new DeviceQueueManager(config.ACTION_QUEUE_IDLE_MS);

  const server = new Server(
    {
      name: config.MCP_GATEWAY_NAME,
      version: config.MCP_GATEWAY_VERSION
    },
    {
      capabilities: {
        tools: {}
      }
    }
  );

  const defaultInputSchema = {
    type: "object",
    properties: {},
    additionalProperties: true
  } as const;

  server.setRequestHandler(ListToolsRequestSchema, async () => ({
    tools: [
      { name: "list_devices", description: "List Android and iOS devices", inputSchema: defaultInputSchema },
      { name: "get_active_app", description: "Get foreground app for a device", inputSchema: defaultInputSchema },
      { name: "get_ui_tree", description: "Get minimal UI tree page by snapshot", inputSchema: defaultInputSchema },
      { name: "find_elements", description: "Find nodes via structured selector", inputSchema: defaultInputSchema },
      { name: "tap", description: "Tap by refId, selector, or coordinates", inputSchema: defaultInputSchema },
      { name: "type", description: "Type text after targeting element", inputSchema: defaultInputSchema },
      { name: "swipe", description: "Swipe on screen", inputSchema: defaultInputSchema },
      { name: "screenshot_stream", description: "Collect screenshot stream metadata", inputSchema: defaultInputSchema }
    ]
  }));

  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    const name = request.params.name;
    const args = (request.params.arguments ?? {}) as Record<string, unknown>;

    try {
      switch (name) {
        case "list_devices": {
          const parsed = listDevicesSchema.parse(args);
          const resp = await grpc.listDevices(parsed);
          return asMcpText(shapeDevices(resp));
        }

        case "get_active_app": {
          const parsed = activeAppSchema.parse(args);
          const resp = await grpc.getActiveApp(parsed.device_id, parsed);
          return asMcpText(resp);
        }

        case "get_ui_tree": {
          const parsed = uiTreeSchema.parse(args);
          const resp = await grpc.getUITree(parsed.device_id, parsed);
          return asMcpText(shapeTree(resp, Math.min(config.MAX_UI_NODES, parsed.node_limit ?? config.MAX_UI_NODES)));
        }

        case "find_elements": {
          const parsed = findElementsSchema.parse(args);
          const resp = await grpc.findElements(parsed.device_id, parsed);
          return asMcpText(shapeElements(resp, Boolean(parsed.include_nodes), config.MAX_ELEMENTS));
        }

        case "tap": {
          const parsed = tapSchema.parse(args);
          const timeoutMs = parsed.options?.timeout_ms ?? config.GRPC_TIMEOUT_MS;
          const resp = await queue.enqueue(parsed.device_id, timeoutMs, () => grpc.tap(parsed.device_id, parsed));
          return asMcpText(shapeAction(resp));
        }

        case "type": {
          const parsed = typeSchema.parse(args);
          const timeoutMs = parsed.options?.timeout_ms ?? config.GRPC_TIMEOUT_MS;
          const resp = await queue.enqueue(parsed.device_id, timeoutMs, () => grpc.type(parsed.device_id, parsed));
          return asMcpText(shapeAction(resp));
        }

        case "swipe": {
          const parsed = swipeSchema.parse(args);
          const timeoutMs = parsed.options?.timeout_ms ?? config.GRPC_TIMEOUT_MS;
          const resp = await queue.enqueue(parsed.device_id, timeoutMs, () => grpc.swipe(parsed.device_id, parsed));
          return asMcpText(shapeAction(resp));
        }

        case "screenshot_stream": {
          const parsed = screenshotStreamSchema.parse(args);
          const maxFrames = Math.min(parsed.max_frames ?? config.MAX_STREAM_FRAMES, config.MAX_STREAM_FRAMES);
          const events = await grpc.collectScreenshotFrames(parsed.device_id, { ...parsed, max_frames: maxFrames }, maxFrames);
          return asMcpText(shapeScreenshotEvents(events));
        }

        default:
          throw new Error(`unknown tool: ${name}`);
      }
    } catch (error) {
      logger.error({ err: error, tool: name, args }, "tool call failed");
      throw error;
    }
  });

  const transport = new StdioServerTransport();
  await server.connect(transport);

  const shutdown = () => {
    queue.stop();
    grpc.close();
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);

  logger.info({
    android: config.ANDROID_WORKER_ADDR,
    ios: config.IOS_WORKER_ADDR
  }, "gateway started");
}
