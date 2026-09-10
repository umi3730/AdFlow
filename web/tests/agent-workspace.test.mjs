import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import {
  describeCondition,
  draftSource,
  editorFromAgentDraft,
  warningLabel,
} from '../lib/rule-presentation.ts';
import { compileRuleDraft, prepareAgentPlan } from '../lib/campaign-rules.ts';
import {
  newAgentCampaignInput,
  nextTestPlanNumber,
  testPlanName,
} from '../lib/agent-campaign.ts';

test('New plan input trims fields and uses a seven-day draft period', () => {
  const now = new Date('2026-09-04T02:00:00.000Z');
  const input = newAgentCampaignInput(
    '  策略投放  ',
    ' game-home-banner ',
    now,
  );
  assert.deepEqual(input, {
    name: '策略投放',
    slotId: 'game-home-banner',
    startAt: now.toISOString(),
    endAt: '2026-09-11T02:00:00.000Z',
  });
  assert.equal(input.status, undefined);
  assert.equal(input.targeting, undefined);
  for (const name of ['', ' ', 'a', '中'.repeat(129)])
    assert.throws(() => newAgentCampaignInput(name, 'banner', now));
  for (const slot of ['', ' ', 'a', 'x'.repeat(65)])
    assert.throws(() => newAgentCampaignInput('计划', slot, now));
});

test('Plan defaults fill the first unused number instead of carrying a browser counter', () => {
  assert.equal(testPlanName(nextTestPlanNumber([])), '测试计划 001');
  assert.equal(
    testPlanName(
      nextTestPlanNumber([{ name: '测试计划 009' }, { name: '真实计划 999' }]),
    ),
    '测试计划 001',
  );
  assert.equal(
    nextTestPlanNumber([{ name: '测试计划 001' }, { name: '测试计划 003' }]),
    2,
  );
  assert.equal(nextTestPlanNumber([], NaN), 1);
  assert.equal(nextTestPlanNumber([], -2), 1);
  assert.equal(testPlanName(1000), '测试计划 1000');
});

test('Editable costs are normalized and transferred with cloned targeting, not model defaults', () => {
  const original = {
    targeting: {
      all: [{ tag: 'anime' }],
      any: [],
      none: [{ tag: 'installed_target_game' }],
    },
    dailyBudgetFen: 1000000,
    impressionCostFen: 100,
    frequencyLimit: 3,
  };
  const value = editorFromAgentDraft(original);
  value.dailyBudgetYuan = '25.80';
  value.impressionCostYuan = '0.08';
  value.frequencyLimit = '6';
  const handoff = prepareAgentPlan(value);
  const published = compileRuleDraft(handoff);
  assert.equal(published.dailyBudgetFen, 2580);
  assert.equal(published.impressionCostFen, 8);
  assert.equal(published.frequencyLimit, 6);
  handoff.targeting.all[0].tag = 'strategy_game';
  assert.equal(value.targeting.all[0].tag, 'anime');
  assert.equal(original.dailyBudgetFen, 1000000);
  assert.throws(() => prepareAgentPlan({ ...value, dailyBudgetYuan: '-1' }));
  assert.throws(() => prepareAgentPlan({ ...value, frequencyLimit: '101' }));
});

test('Agent only hands off new plans and exposes three directly editable controls', () => {
  const source = readFileSync(
    new URL('../components/agent-workspace.tsx', import.meta.url),
    'utf8',
  );
  assert.doesNotMatch(
    source,
    /JSON\.stringify|<pre|api\.createCampaign|api\.publishCampaign|selectedDraftCampaign|选择待发布计划/,
  );
  assert.match(source, /onCreatePlan\(prepareAgentPlan\(value\)\)/);
  for (const id of ['agent-budget', 'agent-cost', 'agent-frequency'])
    assert.ok(source.includes('id="' + id + '"'));
  assert.match(
    source,
    /setPrompt\(event\.target\.value\)[\s\S]*?setDraft\(null\)/,
  );
  assert.match(source, /setDraft\(null\)[\s\S]*?await api\.generateRuleDraft/);
});

test('Campaign page attaches imported rules only to the newly created plan and increments on success', () => {
  const source = readFileSync(
    new URL('../components/adflow-console.tsx', import.meta.url),
    'utf8',
  );
  const campaignSource = readFileSync(
    new URL('../components/console/campaigns-view.tsx', import.meta.url),
    'utf8',
  );
  assert.match(source, /setPendingAgentDraft\(structuredClone\(value\)\)/);
  assert.match(source, /setView\('campaigns'\)/);
  assert.match(source, /\[campaign.id\]: structuredClone\(pendingAgentDraft\)/);
  assert.match(
    campaignSource,
    /await api\.createCampaign[\s\S]*?onCreated\(created\)[\s\S]*?setName\(null\)/,
  );
  assert.match(campaignSource, /campaignDrafts\[selectedCampaign.id\]/);
  assert.match(source, /defaultName=\{testPlanName\(testSequence\)\}/);
});

test('Chinese summaries preserve unknown IDs and legacy field identity', () => {
  assert.equal(describeCondition({ tag: 'anime' }), '二次元兴趣');
  assert.equal(describeCondition({ tag: 'custom_tag' }), '标签「custom_tag」');
  assert.equal(
    describeCondition({ field: 'device', op: 'eq', value: 'android' }),
    '设备 等于 安卓',
  );
  assert.equal(
    describeCondition({ field: 'score', op: 'gte', value: '80' }),
    '活跃分数 大于等于 80',
  );
  assert.equal(
    describeCondition({ field: 'platform', op: 'eq', value: 'android' }),
    '平台（独立字段） 等于 android',
  );
});

test('Generated rules are copied without silently renaming fields', () => {
  const draft = {
    targeting: { all: [{ field: 'platform', op: 'eq', value: 'android' }] },
    dailyBudgetFen: 12345,
    impressionCostFen: 9,
    frequencyLimit: 3,
  };
  const editor = editorFromAgentDraft(draft);
  assert.equal(editor.dailyBudgetYuan, '123.45');
  assert.equal(editor.impressionCostYuan, '0.09');
  editor.targeting.all[0].field = 'device';
  assert.equal(draft.targeting.all[0].field, 'platform');
});

test('Source labels distinguish fallback and unknown warnings are not hidden', () => {
  assert.equal(
    draftSource({ fallback: true, provider: 'local-mock' }),
    '已降级：本地规则解析',
  );
  assert.match(draftSource({ provider: 'local-mock' }), /非大模型/);
  assert.equal(
    draftSource({ provider: 'openai-compatible', model: 'deepseek-v4-flash' }),
    '模型：deepseek-v4-flash',
  );
  assert.match(
    warningLabel(
      'Budget and cost are conservative defaults; adjust as needed.',
    ),
    /模型补全/,
  );
  assert.equal(
    warningLabel('Unknown safety warning'),
    'Unknown safety warning',
  );
});
