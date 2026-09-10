import {
  lstat,
  mkdir,
  mkdtemp,
  readdir,
  realpath,
  rm,
  unlink,
} from 'node:fs/promises';
import { basename, dirname, join, resolve } from 'node:path';
import { run, webRoot } from './runtime.mjs';

const root = join(webRoot, 'work', 'e2e');
await mkdir(root, { recursive: true });
const directory = await mkdtemp(join(root, 'run-'));
try {
  await run(
    process.execPath,
    [
      join(webRoot, 'node_modules', '@playwright', 'test', 'cli.js'),
      'test',
      ...process.argv.slice(2),
    ],
    {
      cwd: webRoot,
      env: { ...process.env, ADFLOW_E2E_RUN_DIR: directory },
    },
  );
} catch {
  process.exitCode = 1;
} finally {
  await cleanup();
}

async function cleanup() {
  // Only remove this run's generated directory. Unlink dependency junctions
  // explicitly before recursive removal so the shared installation stays intact.
  const actualRoot = await realpath(root);
  const actualDirectory = await realpath(directory);
  if (
    dirname(actualDirectory) !== actualRoot ||
    !/^run-[a-zA-Z0-9_-]+$/.test(basename(directory))
  ) {
    throw new Error('Refusing to clean a directory outside the E2E workspace');
  }
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const dependencyLink = resolve(directory, entry.name, 'node_modules');
    try {
      if ((await lstat(dependencyLink)).isSymbolicLink())
        await unlink(dependencyLink);
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
    }
  }
  await rm(actualDirectory, {
    recursive: true,
    force: true,
    maxRetries: 5,
    retryDelay: 200,
  });
}
