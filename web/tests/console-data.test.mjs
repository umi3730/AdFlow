import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import ts from 'typescript';

function renderHook(api) {
  const source = readFileSync(
    new URL('../hooks/use-console-data.ts', import.meta.url),
    'utf8',
  );
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;
  const slots = [];
  let cursor = 0;
  const exports = {};
  runInNewContext(compiled, {
    exports,
    require(name) {
      if (name === '@/lib/api') return { api };
      if (name !== 'react') throw new Error('Unexpected dependency ' + name);
      return {
        useRef(value) {
          const index = cursor++;
          return (slots[index] ??= { current: value });
        },
        useState(value) {
          const index = cursor++;
          slots[index] ??= {
            value: typeof value === 'function' ? value() : value,
            set(next) {
              slots[index].value =
                typeof next === 'function' ? next(slots[index].value) : next;
            },
          };
          return [slots[index].value, slots[index].set];
        },
        useCallback: (callback) => callback,
      };
    },
  });
  return () => {
    cursor = 0;
    return exports.useConsoleData();
  };
}

function deferred() {
  let resolve;
  const promise = new Promise((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

test('Console refresh coalesces concurrent callers and reads again after a queued mutation', async () => {
  const first = deferred();
  let calls = 0;
  const render = renderHook({
    listCampaigns: () =>
      ++calls === 1
        ? first.promise
        : Promise.resolve({ items: [{ id: 'new' }] }),
    listCreatives: async () => ({
      items: [{ status: 'ACTIVE' }, { status: 'DISABLED' }],
    }),
    metrics: async (id) => ({ impressions: id === 'new' ? 2 : 1 }),
  });
  assert.equal(calls, 0);
  const pending = render().refresh();
  assert.equal(render().refresh(), pending);
  assert.equal(render().refresh(), pending);
  assert.equal(calls, 1);
  assert.equal(render().refreshing, true);
  first.resolve({ items: [{ id: 'old' }] });
  await pending;
  const data = render();
  assert.equal(calls, 2);
  assert.equal(data.campaigns[0].id, 'new');
  assert.equal(data.campaigns[0].activeCreativeCount, 1);
  assert.equal(data.metrics.new.impressions, 2);
  assert.equal(data.refreshing, false);
  assert.equal(data.connected, true);
});

test('Creative availability failure remains unknown and does not hide campaign metrics', async () => {
  const render = renderHook({
    listCampaigns: async () => ({ items: [{ id: 'plan' }] }),
    listCreatives: async () => {
      throw new Error('creative store unavailable');
    },
    metrics: async () => ({ impressions: 5 }),
  });
  await render().refresh();
  assert.equal(render().campaigns[0].activeCreativeCount, null);
  assert.equal(render().metrics.plan.impressions, 5);
  assert.equal(render().connected, true);
});

test('A failed refresh releases the flight so a later retry can recover', async () => {
  let fail = true;
  const render = renderHook({
    listCampaigns: async () => {
      if (fail) throw new Error('offline');
      return { items: [] };
    },
    listCreatives: async () => ({ items: [] }),
    metrics: async () => ({}),
  });
  await render().refresh();
  assert.equal(render().refreshing, false);
  assert.equal(render().hasCheckedConnection, true);
  assert.equal(render().connected, false);
  fail = false;
  await render().refresh();
  assert.equal(render().connected, true);
  assert.notEqual(render().lastUpdatedAt, null);
});
