import { cp, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import {
  apiOrigin,
  appOrigin,
  assertFreePort,
  isolatedEnvironment,
  serve,
  temporaryDirectory,
  webRoot,
} from './runtime.mjs';

await assertFreePort(4173);
const directory = await temporaryDirectory('app-');
// A separate checkout copy gives Vinext, Vite and Wrangler their own locks,
// caches and generated files. Do not copy any local environment files.
for (const name of [
  'app',
  'components',
  'hooks',
  'lib',
  'public',
  '.openai',
  'tsconfig.json',
  'next.config.ts',
  'package.json',
]) {
  await cp(join(webRoot, name), join(directory, name), { recursive: true });
}
await cp(
  join(webRoot, 'vite.config.ts'),
  join(directory, 'vite.application.config.ts'),
);
await symlink(
  join(webRoot, 'node_modules'),
  join(directory, 'node_modules'),
  process.platform === 'win32' ? 'junction' : 'dir',
);
await writeFile(
  join(directory, 'vite.config.ts'),
  [
    "import { defineConfig, mergeConfig } from 'vite';",
    "import applicationConfig from './vite.application.config.ts';",
    'export default defineConfig(async () => mergeConfig(await applicationConfig(), {',
    '  cacheDir: ".vite-e2e",',
    '  server: { host: "127.0.0.1", port: 4173, strictPort: true,',
    '    proxy: { "/v1": { target: "' + apiOrigin + '", changeOrigin: true } }',
    '  }',
    '}));',
  ].join('\n'),
);
serve(
  process.execPath,
  [
    join(webRoot, 'node_modules', 'vinext', 'dist', 'cli.js'),
    'dev',
    '--host',
    '127.0.0.1',
    '--port',
    '4173',
  ],
  {
    cwd: directory,
    env: { ...isolatedEnvironment(), NEXT_PUBLIC_ADFLOW_API_URL: appOrigin },
  },
);
