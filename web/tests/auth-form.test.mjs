import test from 'node:test';
import assert from 'node:assert/strict';
import { demoLoginDefaults, registrationError } from '../lib/auth-form.ts';

test('demo defaults prefill both credentials; other environments are empty', () => {
  assert.deepEqual(demoLoginDefaults(true), {
    username: 'admin',
    password: 'adflow-admin',
  });
  assert.deepEqual(demoLoginDefaults(false), { username: '', password: '' });
});
test('registration validation checks names, confirmation, and bcrypt byte limit', () => {
  assert.equal(
    registrationError('Alice', 'demo-password', 'demo-password'),
    '',
  );
  assert.ok(registrationError('bad name', 'demo-password', 'demo-password'));
  assert.ok(registrationError('alice', 'demo-password', 'different'));
  assert.ok(registrationError('alice', '中'.repeat(25), '中'.repeat(25)));
  assert.equal(
    registrationError('alice', '中'.repeat(24), '中'.repeat(24)),
    '',
  );
});
