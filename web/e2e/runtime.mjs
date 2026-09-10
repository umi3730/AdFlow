import { spawn } from 'node:child_process';
import { mkdir, mkdtemp } from 'node:fs/promises';
import { createServer } from 'node:net';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const webRoot = fileURLToPath(new URL('../', import.meta.url));
export const repoRoot = fileURLToPath(new URL('../../', import.meta.url));
export const apiOrigin = 'http://127.0.0.1:18082';
export const appOrigin = 'http://127.0.0.1:4173';

export function isolatedEnvironment(source = process.env) {
  return Object.fromEntries(
    Object.entries(source).filter(
      ([key]) =>
        !/^(ADFLOW_|NEXT_PUBLIC_|VINEXT_|WRANGLER_|MINIFLARE_)/i.test(key) &&
        key.toUpperCase() !== 'NODE_OPTIONS',
    ),
  );
}

export async function assertFreePort(port) {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen({ host: '127.0.0.1', port, exclusive: true }, resolve);
  });
  await new Promise((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
}

export async function temporaryDirectory(prefix) {
  const base = join(webRoot, 'work', 'e2e');
  await mkdir(base, { recursive: true });
  const runDirectory = process.env.ADFLOW_E2E_RUN_DIR;
  if (!runDirectory) return mkdtemp(join(base, prefix));
  if (
    dirname(resolve(runDirectory)) !== resolve(base) ||
    !/^run-[a-zA-Z0-9_-]+$/.test(basename(runDirectory))
  ) {
    throw new Error('Invalid isolated E2E run directory');
  }
  return mkdtemp(join(runDirectory, prefix));
}

export function run(command, args, options) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      ...options,
      stdio: 'inherit',
      windowsHide: true,
    });
    child.once('error', reject);
    child.once('exit', (code, signal) => {
      if (code === 0) resolve();
      else reject(new Error(command + ' exited with ' + (code ?? signal)));
    });
  });
}

export function serve(command, args, options) {
  const child = spawn(command, args, {
    ...options,
    stdio: 'inherit',
    windowsHide: true,
  });
  child.once('error', (error) => {
    console.error(error);
    process.exit(1);
  });
  child.once('exit', (code) => process.exit(code ?? 0));
  for (const signal of ['SIGTERM', 'SIGINT']) {
    process.once(signal, () => child.kill(signal));
  }
}
