import { randomUUID } from "node:crypto";

export type QueueTask<T> = (signal: AbortSignal) => Promise<T>;

class DeviceQueue {
  private tail: Promise<void> = Promise.resolve();
  private depth = 0;
  private lastUsed = Date.now();

  enqueue<T>(timeoutMs: number, task: QueueTask<T>): Promise<T> {
    this.depth += 1;

    const run = async (): Promise<T> => {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), timeoutMs);

      try {
        return await task(controller.signal);
      } finally {
        clearTimeout(timer);
        this.depth -= 1;
        this.lastUsed = Date.now();
      }
    };

    const next = this.tail.then(run, run);
    this.tail = next.then(() => undefined, () => undefined);
    return next;
  }

  isIdle(idleMs: number): boolean {
    return this.depth === 0 && Date.now() - this.lastUsed > idleMs;
  }

  getDepth(): number {
    return this.depth;
  }
}

export class DeviceQueueManager {
  private readonly queues = new Map<string, DeviceQueue>();
  private readonly idleMs: number;
  private readonly cleanupTimer: NodeJS.Timeout;

  constructor(idleMs: number) {
    this.idleMs = idleMs;
    this.cleanupTimer = setInterval(() => this.cleanup(), Math.max(10000, Math.floor(idleMs / 2)));
    this.cleanupTimer.unref();
  }

  enqueue<T>(deviceId: string, timeoutMs: number, task: QueueTask<T>): Promise<T> {
    const queue = this.getOrCreate(deviceId);
    return queue.enqueue(timeoutMs, task);
  }

  queueDepth(deviceId: string): number {
    return this.queues.get(deviceId)?.getDepth() ?? 0;
  }

  stop(): void {
    clearInterval(this.cleanupTimer);
  }

  newActionId(): string {
    return randomUUID();
  }

  private getOrCreate(deviceId: string): DeviceQueue {
    const existing = this.queues.get(deviceId);
    if (existing) return existing;
    const created = new DeviceQueue();
    this.queues.set(deviceId, created);
    return created;
  }

  private cleanup(): void {
    for (const [deviceId, queue] of this.queues.entries()) {
      if (queue.isIdle(this.idleMs)) {
        this.queues.delete(deviceId);
      }
    }
  }
}
