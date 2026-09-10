import { join } from 'node:path';
import {
  assertFreePort,
  isolatedEnvironment,
  repoRoot,
  run,
  serve,
  temporaryDirectory,
} from './runtime.mjs';

// Never inherit the operator's adapters, credentials or .env file.
await assertFreePort(18082);
const directory = await temporaryDirectory('api-');
const binary = join(
  directory,
  process.platform === 'win32' ? 'adflow.exe' : 'adflow',
);
const env = {
  ...isolatedEnvironment(),
  ADFLOW_ENV: 'test',
  ADFLOW_HTTP_ADDR: '127.0.0.1:18082',
  ADFLOW_CAMPAIGN_REPOSITORY: 'memory',
  ADFLOW_DECISION_STORE: 'memory',
  ADFLOW_PROFILE_STORE: 'memory',
  ADFLOW_RESERVATION_ADAPTER: 'memory',
  ADFLOW_DECISION_RATE_LIMITER: 'memory',
  ADFLOW_AUDIT_STORE: 'memory',
  ADFLOW_AUTH_STORE: 'memory',
  ADFLOW_EVENT_TRANSPORT: 'sync',
  ADFLOW_AGENT_PROVIDER: 'mock',
  ADFLOW_DEMO_DATA: 'false',
  ADFLOW_AUTH_ENABLED: 'false',
  ADFLOW_REGISTRATION_ENABLED: 'false',
};
await run('go', ['build', '-o', binary, './cmd/api'], { cwd: repoRoot, env });
serve(binary, [], { cwd: repoRoot, env });
