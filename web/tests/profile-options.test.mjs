import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import {
  profileFields,
  profileTags,
  toggleProfileTag,
  fieldOperators,
  validateFieldCondition,
  fieldValueOptions,
  profileTagLabel,
  profileTagID,
  profileDeviceLabel,
  addProfileTags,
} from '../lib/profile-options.ts';

test('Chinese tag labels and filters preserve canonical and custom identifiers', () => {
  assert.equal(profileTagLabel('adflow_demo'), '演示用户');
  assert.equal(profileTagLabel('demo_excluded'), '演示排除人群');
  assert.equal(profileTagLabel('gaming_interest'), '游戏兴趣');
  assert.equal(profileTagLabel('my_custom_tag'), 'my_custom_tag');
  assert.equal(profileTagID(' 数码兴趣 '), 'tech_interest');
  assert.equal(profileTagID('自定义人群'), '自定义人群');
  assert.equal(
    addProfileTags('custom,tech_interest', '数码兴趣，演示用户,自定义人群'),
    'custom,tech_interest,adflow_demo,自定义人群',
  );
  assert.equal(
    addProfileTags('数码兴趣,custom', '游戏兴趣'),
    '数码兴趣,custom,gaming_interest',
  );
  assert.equal(profileDeviceLabel('android'), '安卓');
  assert.equal(profileDeviceLabel('custom-device'), 'custom-device');
  assert.equal(describeCondition({ tag: 'adflow_demo' }), '演示用户');
});
import { describeCondition } from '../lib/rule-presentation.ts';
import {
  nextTestUserNumber,
  profileSaveMode,
  testUserID,
} from '../lib/profile-defaults.ts';

test('New profile IDs fill current catalog gaps instead of using a saved floor', () => {
  assert.equal(testUserID(nextTestUserNumber([])), 'user-0001');
  assert.equal(
    testUserID(
      nextTestUserNumber([{ userId: 'user-0001' }, { userId: 'custom-99' }]),
    ),
    'user-0002',
  );
  assert.equal(nextTestUserNumber([{ userId: 'user-0009' }], 15), 1);
  assert.equal(nextTestUserNumber([], NaN), 1);
  assert.equal(testUserID(10000), 'user-10000');
});

test('Default ID does not replace selected or manually typed users; creation alone advances the sequence', () => {
  const source = readFileSync(
    new URL('../components/profile-workspace.tsx', import.meta.url),
    'utf8',
  );
  assert.match(
    source,
    /userIDOverride \?\? selected\?\.userId \?\? testUserID\(userSequence\)/,
  );
  assert.match(source, /await api\.putProfile[\s\S]*?setNumberingProfiles\(/);
  assert.match(source, /setUserID\(null\)/);
  assert.match(source, /await api\.getProfile\(id\)/);
});

test('Saving profiles resets to new mode and ID edits are explicit copies, never silent renames', () => {
  assert.equal(profileSaveMode(undefined, 'user-0001'), 'create');
  assert.equal(profileSaveMode('user-0001', ' user-0001 '), 'update');
  assert.equal(profileSaveMode('user-0001', 'user-0002'), 'copy');
  const source = readFileSync(
    new URL('../components/profile-workspace.tsx', import.meta.url),
    'utf8',
  );
  const save = source.slice(
    source.indexOf('async function save('),
    source.indexOf('\n  return ('),
  );
  assert.match(save, /await api\.putProfile[\s\S]*?create\(\);\s*setMessage/);
  assert.doesNotMatch(save, /setSelected\(profile\)|deleteProfile/);
  assert.match(save, /if \(saveMode !== 'update'\) \{\s*let exists/);
  assert.doesNotMatch(source, /readOnly=\{Boolean\(selected\)\}/);
  assert.match(source, /另存为新用户/);
});

test('Decision user options show only user IDs, not tags or device metadata', () => {
  const source = readFileSync(
    new URL('../components/adflow-console.tsx', import.meta.url),
    'utf8',
  );
  const option = source.match(
    /<option key=\{item.userId\} value=\{item.userId\}>([\s\S]*?)<\/option>/,
  );
  assert.ok(option);
  assert.equal(option[1].trim(), '{item.userId}');
});

test('Shared profile catalog has unique canonical IDs and usable field samples', () => {
  assert.equal(profileTags.length, 5);
  assert.deepEqual(
    profileFields.map((field) => field.id),
    ['device', 'score', 'age', 'member_level', 'channel'],
  );
  assert.equal(
    new Set(profileTags.map((tag) => tag.id)).size,
    profileTags.length,
  );
  assert.equal(
    new Set(profileFields.map((field) => field.id)).size,
    profileFields.length,
  );
  for (const tag of profileTags)
    assert.equal(describeCondition({ tag: tag.id }), tag.label);
  for (const field of profileFields)
    assert.equal(typeof field.sample, 'string');
});

test('Frontend and Go use the same typed-condition contract fixtures', () => {
  const cases = JSON.parse(
    readFileSync(
      new URL(
        '../../internal/profile/schema/testdata/conditions.json',
        import.meta.url,
      ),
      'utf8',
    ),
  );
  for (const row of cases) {
    const check = () => validateFieldCondition(row.field, row.op, row.value);
    if (row.valid) assert.doesNotThrow(check, JSON.stringify(row));
    else assert.throws(check, undefined, JSON.stringify(row));
  }
  assert.deepEqual(fieldOperators('country'), ['eq', 'in']);
  assert.deepEqual(fieldOperators('region'), ['eq', 'in']);
  assert.deepEqual(fieldOperators('channel'), ['eq', 'in']);
  assert.deepEqual(fieldOperators('score'), ['eq', 'in', 'gte', 'lte']);
  assert.deepEqual(fieldOperators('age'), ['eq', 'in', 'gte', 'lte']);
  assert.deepEqual(fieldOperators('member_level'), ['eq', 'in']);
});

test('General audience dictionaries use readable labels and fixed values', () => {
  assert.deepEqual(
    profileTags.map((tag) => tag.id),
    [
      'tech_interest',
      'gaming_interest',
      'active_7d',
      'new_user',
      'paying_user',
    ],
  );
  assert.deepEqual(
    fieldValueOptions('member_level').map((item) => item.value),
    ['basic', 'silver', 'gold', 'diamond'],
  );
  assert.deepEqual(
    fieldValueOptions('channel').map((item) => item.value),
    ['organic', 'paid', 'referral'],
  );
  assert.equal(
    describeCondition({ field: 'member_level', op: 'in', value: 'basic,gold' }),
    '会员等级 属于 普通会员、黄金会员',
  );
  assert.equal(
    describeCondition({ field: 'channel', op: 'eq', value: 'organic' }),
    '来源渠道 等于 自然访问',
  );
});

test('Rule editor uses themed selectors instead of native datalist menus', () => {
  const editor = readFileSync(
    new URL('../components/campaign-rule-dialog.tsx', import.meta.url),
    'utf8',
  );
  const row = readFileSync(
    new URL('../components/rule-condition-input.tsx', import.meta.url),
    'utf8',
  );
  const selector = readFileSync(
    new URL('../components/form-select.tsx', import.meta.url),
    'utf8',
  );
  assert.doesNotMatch(editor + row, /<datalist|<select/);
  assert.match(selector, /SelectContent/);
  assert.match(row, /fieldOperators\(field\)/);
});
test('Quick selection supports Chinese commas and preserves custom tags', () => {
  assert.equal(
    toggleProfileTag('custom，anime,anime', 'new_user'),
    'custom,anime,new_user',
  );
  assert.equal(toggleProfileTag('custom,anime', 'anime'), 'custom');
});
test('Deletion is gated by confirmation with errors and no double submit', () => {
  const source = readFileSync(
    new URL('../components/delete-resource-button.tsx', import.meta.url),
    'utf8',
  );
  assert.match(source, /AlertDialog/);
  assert.match(source, /lock.current \|\| deleted/);
  assert.match(source, /await onDelete\(\)/);
  assert.match(source, /确认删除/);
});
