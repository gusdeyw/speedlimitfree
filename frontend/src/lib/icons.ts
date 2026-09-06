import { backend } from './api';

// Only visible rows request icons. No disk cache, downloads, or per-snapshot lookups.
const cache = new Map<string, { data: string; expires: number }>();
const pending = new Map<string, { promise: Promise<string>; users: AbortSignal[] }>();
const queue: (() => Promise<void>)[] = [];
let active = 0;
function drain() {
  while (active < 2 && queue.length) {
    active++;
    void queue.shift()!().finally(() => {
      active--;
      drain();
    });
  }
}
export function loadIcon(path: string, signal: AbortSignal): Promise<string> {
  if (!path || !window.go || signal.aborted) return Promise.resolve('');
  const key = path.toLowerCase();
  const hit = cache.get(key);
  if (hit && hit.expires > Date.now()) {
    cache.delete(key);
    cache.set(key, hit);
    return Promise.resolve(hit.data);
  }
  const current = pending.get(key);
  if (current) {
    current.users.push(signal);
    return current.promise;
  }
  if (queue.length >= 128) return Promise.resolve('');
  let finish!: (data: string) => void;
  const promise = new Promise<string>((resolve) => {
    finish = resolve;
  });
  const job = { promise, users: [signal] };
  pending.set(key, job);
  queue.push(async () => {
    let data = '';
    try {
      if (!job.users.some((s) => !s.aborted)) return;
      data = await backend().AppIcon(path);
      if (!data.startsWith('data:image/png;base64,')) data = '';
      cache.delete(key);
      cache.set(key, { data, expires: Date.now() + (data ? 300000 : 30000) });
      if (cache.size > 256) cache.delete(cache.keys().next().value!);
    } catch {
      /* A generic application icon remains visible. */
    } finally {
      pending.delete(key);
      finish(data);
    }
  });
  drain();
  return promise;
}
