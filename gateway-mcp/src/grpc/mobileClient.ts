import path from "node:path";
import grpc from "@grpc/grpc-js";
import protoLoader from "@grpc/proto-loader";
import { logger } from "../logger.js";

const PROTO_PATH = path.resolve(process.cwd(), "../proto/mobile.proto");

type UnaryCallback<T> = (error: grpc.ServiceError | null, response: T) => void;

type RawClient = grpc.Client & {
  ListDevices(request: unknown, metadata: grpc.Metadata, options: grpc.CallOptions, callback: UnaryCallback<any>): void;
  GetActiveApp(request: unknown, metadata: grpc.Metadata, options: grpc.CallOptions, callback: UnaryCallback<any>): void;
  GetUITree(request: unknown, metadata: grpc.Metadata, options: grpc.CallOptions, callback: UnaryCallback<any>): void;
  FindElements(request: unknown, metadata: grpc.Metadata, options: grpc.CallOptions, callback: UnaryCallback<any>): void;
  Tap(request: unknown, metadata: grpc.Metadata, options: grpc.CallOptions, callback: UnaryCallback<any>): void;
  Type(request: unknown, metadata: grpc.Metadata, options: grpc.CallOptions, callback: UnaryCallback<any>): void;
  Swipe(request: unknown, metadata: grpc.Metadata, options: grpc.CallOptions, callback: UnaryCallback<any>): void;
  ScreenshotStream(request: unknown, metadata?: grpc.Metadata, options?: grpc.CallOptions): grpc.ClientReadableStream<any>;
};

interface ClientConfig {
  androidAddr: string;
  iosAddr: string;
  timeoutMs: number;
  retries: number;
}

type Platform = "PLATFORM_ANDROID" | "PLATFORM_IOS";

export class MobileGrpcRouter {
  private readonly android: RawClient;
  private readonly ios: RawClient;
  private readonly cfg: ClientConfig;
  private readonly devicePlatform = new Map<string, Platform>();

  constructor(cfg: ClientConfig) {
    this.cfg = cfg;
    const pkg = this.loadProtoPackage();
    const ServiceCtor = pkg.mobile.v1.MobileAutomationService;

    this.android = new ServiceCtor(cfg.androidAddr, grpc.credentials.createInsecure()) as RawClient;
    this.ios = new ServiceCtor(cfg.iosAddr, grpc.credentials.createInsecure()) as RawClient;
  }

  async listDevices(request: Record<string, unknown>): Promise<any> {
    const results = await Promise.allSettled([
      this.invoke(this.android, "ListDevices", request),
      this.invoke(this.ios, "ListDevices", request)
    ]);

    const successes = results
      .filter((entry): entry is PromiseFulfilledResult<any> => entry.status === "fulfilled")
      .map((entry) => entry.value);

    if (successes.length === 0) {
      const firstError = (results.find((entry) => entry.status === "rejected") as PromiseRejectedResult | undefined)?.reason;
      throw firstError ?? new Error("all workers are unavailable");
    }

    for (const failure of results.filter((entry): entry is PromiseRejectedResult => entry.status === "rejected")) {
      logger.warn({ err: failure.reason }, "worker listDevices failed; continuing with available workers");
    }

    const devices = successes.flatMap((resp) => resp.devices ?? []);
    for (const device of devices) {
      if (device?.device_id && device?.platform) {
        this.devicePlatform.set(device.device_id as string, device.platform as Platform);
      }
    }

    return {
      devices,
      cache_age_ms: Math.max(...successes.map((resp) => Number(resp.cache_age_ms ?? 0)))
    };
  }

  getActiveApp(deviceId: string, request: Record<string, unknown>): Promise<any> {
    return this.routeToDevice(deviceId, (c) => this.invoke(c, "GetActiveApp", request));
  }

  getUITree(deviceId: string, request: Record<string, unknown>): Promise<any> {
    return this.routeToDevice(deviceId, (c) => this.invoke(c, "GetUITree", request));
  }

  findElements(deviceId: string, request: Record<string, unknown>): Promise<any> {
    return this.routeToDevice(deviceId, (c) => this.invoke(c, "FindElements", request));
  }

  tap(deviceId: string, request: Record<string, unknown>): Promise<any> {
    return this.routeToDevice(deviceId, (c) => this.invoke(c, "Tap", request));
  }

  type(deviceId: string, request: Record<string, unknown>): Promise<any> {
    return this.routeToDevice(deviceId, (c) => this.invoke(c, "Type", request));
  }

  swipe(deviceId: string, request: Record<string, unknown>): Promise<any> {
    return this.routeToDevice(deviceId, (c) => this.invoke(c, "Swipe", request));
  }

  async collectScreenshotFrames(deviceId: string, request: Record<string, unknown>, maxFrames: number): Promise<any[]> {
    const client = await this.resolveClient(deviceId);
    const stream = client.ScreenshotStream(request, new grpc.Metadata(), { deadline: Date.now() + this.cfg.timeoutMs * 10 });

    const events: any[] = [];
    await new Promise<void>((resolve, reject) => {
      stream.on("data", (evt) => {
        events.push(evt);
        const metas = events.filter((e) => e.frame_meta).length;
        if (metas >= maxFrames) {
          stream.cancel();
          resolve();
        }
      });
      stream.on("error", (err) => {
        const grpcErr = err as grpc.ServiceError;
        if (grpcErr.code === grpc.status.CANCELLED) {
          resolve();
          return;
        }
        reject(grpcErr);
      });
      stream.on("end", () => resolve());
    });

    return events;
  }

  close(): void {
    this.android.close();
    this.ios.close();
  }

  private async routeToDevice<T>(deviceId: string, run: (client: RawClient) => Promise<T>): Promise<T> {
    const client = await this.resolveClient(deviceId);
    return run(client);
  }

  private async resolveClient(deviceId: string): Promise<RawClient> {
    const platform = this.devicePlatform.get(deviceId);
    if (platform === "PLATFORM_ANDROID") return this.android;
    if (platform === "PLATFORM_IOS") return this.ios;

    const listed = await this.listDevices({ platform_filter: "PLATFORM_UNSPECIFIED", ready_only: false });
    const found = (listed.devices as Array<any>).find((d) => d.device_id === deviceId);
    if (!found) {
      throw new Error(`device ${deviceId} not found in worker inventory`);
    }

    return found.platform === "PLATFORM_IOS" ? this.ios : this.android;
  }

  private async invoke(client: RawClient, method: keyof RawClient, request: Record<string, unknown>): Promise<any> {
    let lastError: unknown;

    for (let attempt = 0; attempt <= this.cfg.retries; attempt += 1) {
      try {
        const response = await new Promise<any>((resolve, reject) => {
          const deadline = Date.now() + this.cfg.timeoutMs;
          const metadata = new grpc.Metadata();
          const callback: UnaryCallback<any> = (error, resp) => {
            if (error) return reject(error);
            return resolve(resp);
          };

          (client[method] as any)(request, metadata, { deadline }, callback);
        });
        return response;
      } catch (error) {
        lastError = error;
        const code = (error as grpc.ServiceError).code;
        const retryable = code === grpc.status.UNAVAILABLE || code === grpc.status.DEADLINE_EXCEEDED;
        if (!retryable || attempt === this.cfg.retries) {
          break;
        }
        const backoffMs = 20 * Math.pow(2, attempt);
        await new Promise((r) => setTimeout(r, backoffMs));
      }
    }

    logger.error({ err: lastError, method, request }, "grpc invocation failed");
    throw lastError;
  }

  private loadProtoPackage(): any {
    const definition = protoLoader.loadSync(PROTO_PATH, {
      keepCase: true,
      longs: String,
      enums: String,
      defaults: true,
      oneofs: true
    });
    return grpc.loadPackageDefinition(definition);
  }
}
