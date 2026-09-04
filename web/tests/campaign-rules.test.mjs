import assert from 'node:assert/strict';
import test from 'node:test';
import { compileRuleDraft, editorFromCampaign } from '../lib/campaign-rules.ts';

const campaign = {
  id: 'test',
  name: '测试计划',
  slotId: 'banner',
  status: 'PAUSED',
  activeVersion: {
    number: 2,
    targeting: {
      all: [
        { tag: 'anime' },
        { field: 'platform', op: 'eq', value: 'android' },
      ],
      any: [{ tag: 'active_7d' }],
      none: [{ tag: 'installed_target_game' }],
    },
    dailyBudgetFen: 1999,
    impressionCostFen: 1,
    frequencyLimit: 3,
  },
};

test('editing copies current rules without changing published version or field names', () => {
  const editor = editorFromCampaign(campaign);
  assert.equal(editor.targeting.all[1].field, 'platform');
  editor.targeting.all[1].field = 'device';
  assert.equal(campaign.activeVersion.targeting.all[1].field, 'platform');
  assert.equal(campaign.activeVersion.number, 2);
  assert.deepEqual(compileRuleDraft(editor).targeting.none, [
    { tag: 'installed_target_game' },
  ]);
});

test('converts yuan to integer fen exactly and preserves all/any/none', () => {
  const payload = compileRuleDraft(editorFromCampaign(campaign));
  assert.equal(payload.dailyBudgetFen, 1999);
  assert.equal(payload.impressionCostFen, 1);
  assert.deepEqual(payload.targeting, campaign.activeVersion.targeting);
});

test('rejects empty rules, incomplete conditions, invalid money and frequency', () => {
  const editor = editorFromCampaign(campaign);
  for (const dailyBudgetYuan of [
    '-1',
    '0',
    '1.001',
    'Infinity',
    'NaN',
    '1e6',
  ]) {
    assert.throws(() => compileRuleDraft({ ...editor, dailyBudgetYuan }));
  }
  for (const frequencyLimit of ['0', '101', '1.5', '']) {
    assert.throws(() => compileRuleDraft({ ...editor, frequencyLimit }));
  }
  assert.throws(() =>
    compileRuleDraft({ ...editor, impressionCostYuan: '20' }),
  );
  assert.throws(() =>
    compileRuleDraft({ ...editor, targeting: { all: [], any: [], none: [] } }),
  );
  assert.throws(() =>
    compileRuleDraft({
      ...editor,
      targeting: { all: [{ tag: ' ' }], any: [], none: [] },
    }),
  );
  assert.throws(() =>
    compileRuleDraft({
      ...editor,
      targeting: {
        all: [{ field: 'score', op: 'gte', value: 'abc' }],
        any: [],
        none: [],
      },
    }),
  );
});
