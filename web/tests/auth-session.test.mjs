import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { api, ApiError } from '../lib/api.ts';
import {
  allowDemoAutoLogin,
  pauseDemoLogin,
  resumeDemoLogin,
  demoLoginPaused,
} from '../lib/demo-login.ts';

test('Demo auto-login requires explicit opt-in and both page and API on loopback', () => {
  assert.equal(
    allowDemoAutoLogin(
      true,
      'http://127.0.0.1:3000/',
      'http://localhost:18080',
    ),
    true,
  );
  assert.equal(
    allowDemoAutoLogin(false, 'http://localhost', 'http://localhost'),
    false,
  );
  assert.equal(
    allowDemoAutoLogin(true, 'https://example.com', 'http://localhost'),
    false,
  );
  assert.equal(
    allowDemoAutoLogin(true, 'http://localhost', 'https://api.example.com'),
    false,
  );
  assert.equal(
    allowDemoAutoLogin(true, 'http://localhost.evil', 'http://localhost'),
    false,
  );
  assert.equal(
    allowDemoAutoLogin(true, 'file:///test', 'http://localhost'),
    false,
  );
  assert.equal(
    allowDemoAutoLogin(true, 'http://localhost', 'http://u:p@localhost'),
    false,
  );
  assert.equal(
    allowDemoAutoLogin(true, 'http://localhost', 'http://localhost', true),
    false,
  );
});

test('Explicit logout suppresses future auto-login, without storing tokens', () => {
  const original = globalThis.sessionStorage;
  const values = new Map();
  globalThis.sessionStorage = {
    setItem: (key, value) => values.set(key, value),
    getItem: (key) => values.get(key) ?? null,
    removeItem: (key) => values.delete(key),
  };
  try {
    assert.equal(demoLoginPaused(), false);
    pauseDemoLogin();
    assert.equal(demoLoginPaused(), true);
    assert.deepEqual([...values.values()], ['true']);
    resumeDemoLogin();
    assert.equal(demoLoginPaused(), false);
  } finally {
    if (original === undefined) delete globalThis.sessionStorage;
    else globalThis.sessionStorage = original;
  }
});

test('Login page has no theme artwork or redundant token explanation', () => {
  const gate = readFileSync(
    new URL('../components/auth-gate.tsx', import.meta.url),
    'utf8',
  );
  assert.doesNotMatch(gate, /MygoArtwork|令牌仅保留|BLUE NOTES/);
  assert.match(gate, /demoLoginDefaults/);
  assert.match(gate, /\.authOptions\(\)/);
  assert.doesNotMatch(gate, /NEXT_PUBLIC_ADFLOW_DEMO_AUTO_LOGIN/);
  assert.match(gate, /没有账号？注册账号/);
  assert.match(gate, /setSession\(issued\)/);
});
import {
  setSession,
  clearSession,
  sessionToken,
  sessionRevision,
  sessionExpiry,
  expireSessionIfCurrent,
  subscribeSession,
  roleAllows,
} from '../lib/auth-session.ts';

const issued = (accessToken = 'synthetic-test-token') => ({
  accessToken,
  tokenType: 'Bearer',
  expiresAt: new Date(Date.now() + 60000).toISOString(),
  principal: { userId: 'admin', username: 'admin', role: 'admin' },
});

test('Role hierarchy fails closed and distinguishes administrator from operator', () => {
  assert.equal(roleAllows('viewer', 'operator'), false);
  assert.equal(roleAllows('operator', 'admin'), false);
  assert.equal(roleAllows('operator', 'viewer'), true);
  assert.equal(roleAllows('admin', 'admin'), true);
  assert.equal(roleAllows(undefined, 'viewer'), false);
  assert.equal(roleAllows('unknown', 'viewer'), false);
});

test('In-memory tokens expire, clear listeners and ignore obsolete 401s', () => {
  clearSession();
  let updates = 0;
  const unsubscribe = subscribeSession(() => updates++);
  setSession(issued('old'));
  const old = sessionRevision();
  setSession(issued('new'));
  expireSessionIfCurrent(old);
  assert.equal(sessionToken(), 'new');
  assert.equal(sessionToken(sessionExpiry() + 1), '');
  assert.equal(updates, 3);
  unsubscribe();
  assert.throws(() => setSession({ ...issued(), expiresAt: 'invalid' }));
  assert.throws(() =>
    setSession({ ...issued(), expiresAt: new Date(0).toISOString() }),
  );
});

test('API adds Bearer on protected calls, omits login token, handles 403 vs 401', async () => {
  const originalFetch = globalThis.fetch;
  try {
    setSession(issued());
    globalThis.fetch = async (_, init) => {
      assert.equal(
        init.headers.get('Authorization'),
        'Bearer synthetic-test-token',
      );
      return Response.json({ items: [] });
    };
    await api.listCampaigns();
    globalThis.fetch = async (_, init) => {
      assert.equal(init.headers.get('Authorization'), null);
      return Response.json(
        { error: { code: 'invalid_credentials' } },
        { status: 401 },
      );
    };
    await assert.rejects(api.login('admin', 'wrong-password'), ApiError);
    assert.equal(sessionToken(), 'synthetic-test-token');
    globalThis.fetch = async () => Response.json({}, { status: 403 });
    await assert.rejects(api.listCampaigns(), (err) => err.status === 403);
    assert.equal(sessionToken(), 'synthetic-test-token');
    globalThis.fetch = async () => Response.json({}, { status: 401 });
    await assert.rejects(api.listCampaigns(), (err) => err.status === 401);
    assert.equal(sessionToken(), '');
    setSession(issued('prior'));
    globalThis.fetch = async () => {
      setSession(issued('fresh'));
      return Response.json({}, { status: 401 });
    };
    await assert.rejects(api.listCampaigns());
    assert.equal(sessionToken(), 'fresh');
  } finally {
    globalThis.fetch = originalFetch;
    clearSession();
  }
});

test('Console is auth gated, secrets never persist, write-only tools are role gated', () => {
  const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
  assert.match(read('../app/page.tsx'), /<AuthGate>/);
  assert.doesNotMatch(
    read('../lib/auth-session.ts') + read('../components/auth-gate.tsx'),
    /localStorage|sessionStorage/,
  );
  assert.match(read('../hooks/use-adflow-tools.ts'), /if \(canOperate\)/);
  assert.match(read('../components/campaign-rule-dialog.tsx'), /canAdmin &&/);
  assert.match(
    read('../components/delete-resource-button.tsx'),
    /if \(!canAdmin\) return null/,
  );
  assert.match(
    read('../components/adflow-console.tsx'),
    /<RoleGate minimum="admin">/,
  );
});

test('Operations distinguishes disabled mode, unknown state and absent Kafka samples', () => {
  const source = readFileSync(
    new URL('../components/adflow-console.tsx', import.meta.url),
    'utf8',
  );
  assert.match(source, /api.operationsMode\(\)/);
  assert.match(source, /eventTransport === 'sync'/);
  assert.match(source, /尚无有效分区采样/);
  assert.doesNotMatch(source, /投递链路健康/);
});
