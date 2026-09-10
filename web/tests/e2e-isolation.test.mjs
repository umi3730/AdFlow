import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:net';
import { assertFreePort, isolatedEnvironment } from '../e2e/runtime.mjs';

test('E2E services do not inherit live adapters, credentials or API targets', () => {
  const clean = isolatedEnvironment({
    PATH: 'toolchain',
    ADFLOW_MYSQL_DSN: 'live-database',
    adflow_profile_store: 'mysql-redis',
    ADFLOW_AGENT_API_KEY: 'not-a-real-key',
    NEXT_PUBLIC_ADFLOW_API_URL: 'https://example.test',
    VINEXT_NO_DEV_LOCK: '1',
    WRANGLER_LOG_PATH: 'shared-runtime',
    MINIFLARE_REGISTRY_PATH: 'shared-registry',
    NODE_OPTIONS: '--require=unexpected-preload',
  });
  assert.deepEqual(clean, { PATH: 'toolchain' });
});

test('E2E startup refuses an occupied port instead of reusing the listener', async () => {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  try {
    const address = server.address();
    assert.equal(typeof address, 'object');
    await assert.rejects(assertFreePort(address.port), { code: 'EADDRINUSE' });
  } finally {
    await new Promise((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
});
