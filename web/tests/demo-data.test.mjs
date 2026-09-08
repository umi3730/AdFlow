import assert from 'node:assert/strict';
import test from 'node:test';
import { loadDemoProfiles, demoUserCases } from '../lib/demo-data.ts';
import { displayCreativeURL } from '../lib/demo-creatives.ts';

test('Demo case reads current records and skips deleted profiles without restoring them', async () => {
  const changed = {
    userId: demoUserCases[0].userId,
    tags: ['user-edited'],
    fields: { device: 'ios' },
  };
  const calls = [];
  const profiles = await loadDemoProfiles(async (id) => {
    calls.push(id);
    if (id === changed.userId) return changed;
    throw { status: 404 };
  });
  assert.deepEqual(
    calls,
    demoUserCases.map((item) => item.userId),
  );
  assert.deepEqual(profiles, [changed]);
  assert.deepEqual(
    await loadDemoProfiles(async () => {
      throw { status: 404 };
    }),
    [],
  );
});

test('Demo case does not conceal authentication or service failures as deleted users', async () => {
  for (const status of [401, 403, 500])
    await assert.rejects(
      loadDemoProfiles(async () => {
        throw { status };
      }),
      (error) => error.status === status,
    );
});

test('Seeded creative resolves locally on any frontend origin, without remapping unknown URLs', () => {
  const path = '/demo-assets/bangdream/anon.webp';
  assert.equal(displayCreativeURL('https://adflow.invalid' + path), path);
  for (const input of [
    'https://example.com' + path,
    'https://adflow.invalid/unknown.png',
    'https://adflow.invalid' + path + '?x=1',
    path,
  ])
    assert.equal(displayCreativeURL(input), input);
});
