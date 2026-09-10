import test from 'node:test';
import assert from 'node:assert/strict';
import {
  profileDisplayName,
  isDemoProfile,
} from '../lib/profile-presentation.ts';
import {
  profileTagID,
  profileTagInfo,
  profileTagLabel,
  ruleTagOptions,
} from '../lib/profile-options.ts';

test('Rule selectors translate existing demo and legacy tags without changing their values', () => {
  for (const [id, label] of [
    ['auction_demo', '竞价人群'],
    ['adflow_demo', '基础演示人群'],
    ['demo_excluded', '排除演示人群'],
    ['anime', '二次元兴趣'],
  ]) {
    const options = ruleTagOptions(id);
    assert.equal(options.length, 6);
    assert.deepEqual(options.at(-1), { value: id, label });
  }
  assert.equal(ruleTagOptions('tech_interest').length, 5);
  assert.equal(ruleTagOptions('unknown_custom_tag').length, 5);
  assert.equal(ruleTagOptions('').length, 5);
});

test('Bundled users have purpose labels without renaming custom or case-sensitive IDs', () => {
  const examples = [
    ['demo-auction-user', '竞价演示用户'],
    ['demo-user-match', '基础投放用户'],
    ['demo-user-excluded', '排除规则用户'],
    ['demo-user-miss', '兴趣对照用户'],
  ];
  for (const [id, label] of examples) {
    assert.equal(profileDisplayName(id), label);
    assert.equal(isDemoProfile(id), true);
  }
  for (const id of [
    'user-0001',
    'DEMO-USER-MATCH',
    'constructor',
    '__proto__',
    '自定义用户',
  ]) {
    assert.equal(profileDisplayName(id), id);
    assert.equal(isDemoProfile(id), false);
  }
});

test('Updated and previous Chinese tag labels resolve to the same canonical tag', () => {
  for (const [id, previous] of [
    ['adflow_demo', '演示用户'],
    ['auction_demo', '竞价演示用户'],
    ['demo_excluded', '演示排除人群'],
  ]) {
    assert.equal(profileTagID(previous), id);
    assert.equal(profileTagID(profileTagLabel(id)), id);
    assert.equal(profileTagID(id), id);
  }
});

test('Tag presentation distinguishes interests, behavior and demo conditions without inferring a decision', () => {
  assert.equal(profileTagInfo('tech_interest').kind, 'interest');
  assert.equal(profileTagInfo('active_7d').kind, 'behavior');
  assert.equal(profileTagInfo('auction_demo').kind, 'demo');
  assert.match(
    profileTagInfo('auction_demo').description,
    /不表示已经命中或成交/,
  );
  assert.equal(profileTagInfo('demo_excluded').kind, 'exclusion');
  assert.match(
    profileTagInfo('demo_excluded').description,
    /仅在计划设置对应排除规则时生效/,
  );
  const custom = profileTagInfo('campaign_specific_tag');
  assert.equal(custom.kind, 'custom');
  assert.equal(custom.label, 'campaign_specific_tag');
});
