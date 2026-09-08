import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { createHash } from 'node:crypto';
import {
  firstAvailableNumber,
  collectNumberingPages,
} from '../lib/available-number.ts';

test('Default numbers reuse gaps, reset for empty data, and ignore arbitrary suffixes', () => {
  const pattern = /^测试计划 (\d+)$/;
  assert.equal(
    firstAvailableNumber(['测试计划 019', '测试计划 020'], pattern),
    1,
  );
  assert.equal(
    firstAvailableNumber(['测试计划 001', '测试计划 003'], pattern),
    2,
  );
  assert.equal(
    firstAvailableNumber(
      ['测试计划 1', '测试计划 001', '测试计划 000', '其他 002'],
      pattern,
    ),
    2,
  );
  assert.equal(
    firstAvailableNumber(['测试计划 99999999999999999999'], pattern),
    1,
  );
  assert.equal(firstAvailableNumber([], pattern), 1);
});

test('Numbering loads beyond the visible first page and does not return partial failures', async () => {
  const all = Array.from({ length: 205 }, (_, i) => ({
    userId: 'user-' + String(i + 1).padStart(4, '0'),
  }));
  const offsets = [];
  const result = await collectNumberingPages(async (offset, limit) => {
    offsets.push(offset);
    return { items: all.slice(offset, offset + limit) };
  });
  assert.deepEqual(offsets, [0, 100, 200]);
  assert.equal(
    firstAvailableNumber(
      result.map((item) => item.userId),
      /^user-(\d+)$/,
    ),
    206,
  );
  await assert.rejects(
    collectNumberingPages(async (offset) => {
      if (offset) throw new Error('offline');
      return { items: all.slice(0, 100) };
    }),
    /offline/,
  );
});

test('All default labels ignore old browser counters and profiles use an unfiltered catalog', () => {
  const main = readFileSync(
    new URL('../components/adflow-console.tsx', import.meta.url),
    'utf8',
  );
  const profile = readFileSync(
    new URL('../components/profile-workspace.tsx', import.meta.url),
    'utf8',
  );
  assert.doesNotMatch(
    main + profile,
    /adflow\.test-(plan|creative|user)-sequence|[Ss]equenceFloor/,
  );
  assert.match(profile, /api.listProfileCatalog\(\)/);
  assert.match(profile, /nextTestUserNumber\(numberingProfiles\)/);
  assert.match(main, /nextTestPlanNumber\(campaigns\)/);
  assert.match(main, /nextTestCreativeNumber\(items\)/);
});

test('Previous theme artwork is retained as an intact archive, not an active UI dependency', () => {
  for (const file of ['adflow-console.tsx', 'auth-gate.tsx']) {
    const active = readFileSync(
      new URL('../components/' + file, import.meta.url),
      'utf8',
    );
    assert.doesNotMatch(
      active,
      /MygoArtwork|mygo-artwork|theme-assets|MyGO!!!!!/,
    );
  }
  const theme = readFileSync(
    new URL('../components/mygo-artwork.tsx', import.meta.url),
    'utf8',
  );
  assert.match(theme, /theme-assets\/mygo\/five-shiramori.jpg/);
  assert.doesNotMatch(theme, /demo-assets|demoCreatives/);
  assert.match(theme, /pixiv.net\/artworks\/107693545/);
  assert.match(theme, /objectFit: 'contain'/);
  const jpg = readFileSync(
    new URL('../public/theme-assets/mygo/five-shiramori.jpg', import.meta.url),
  );
  assert.equal(
    createHash('sha256').update(jpg).digest('hex'),
    'f7b4b165e3554394fa4f28917b7d65aaa31168ba7d15dafe5e05a1063209e4fd',
  );
});

test('Core blue-white palette text combinations meet 4.5:1 contrast', () => {
  const root = readFileSync(
    new URL('../app/globals.css', import.meta.url),
    'utf8',
  )
    .split(':root {')[1]
    .split('}')[0];
  const color = (name) =>
    root.match(new RegExp('--' + name + ': (#[0-9a-f]{6})'))[1];
  const luminance = (hex) =>
    hex
      .slice(1)
      .match(/../g)
      .map((v) => parseInt(v, 16) / 255)
      .map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
      .reduce((sum, v, i) => sum + v * [0.2126, 0.7152, 0.0722][i], 0);
  for (const [fg, bg] of [
    ['foreground', 'background'],
    ['card-foreground', 'card'],
    ['primary-foreground', 'primary'],
    ['muted-foreground', 'card'],
    ['sidebar-primary-foreground', 'sidebar-primary'],
    ['sidebar-foreground', 'sidebar'],
  ]) {
    const a = luminance(color(fg)),
      b = luminance(color(bg));
    assert.ok(
      (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) >= 4.5,
      fg + ' / ' + bg,
    );
  }
});
import {
  filterCampaigns,
  allCampaignFilters,
} from '../lib/campaign-filters.ts';

test('campaign filters combine search, slot and status without mutating records', () => {
  const now = Date.parse('2026-09-07T00:00:00Z');
  const filterAt = (
    items,
    query,
    slot = allCampaignFilters,
    status = allCampaignFilters,
  ) => filterCampaigns(items, query, slot, status, now);
  const campaigns = [
    {
      id: '1',
      name: '测试计划 001',
      slotId: 'game-home-banner',
      status: 'DRAFT',
    },
    {
      id: '2',
      name: '测试计划 002',
      slotId: 'feed-recommendation',
      status: 'ACTIVE',
    },
    {
      id: '3',
      name: 'Summer',
      slotId: 'feed-recommendation',
      status: 'PAUSED',
    },
    { id: '4', name: '旧计划', slotId: 'legacy-slot', status: 'ENDED' },
  ].map((item) => ({
    ...item,
    startAt: '2026-09-06T00:00:00Z',
    endAt: '2026-09-08T00:00:00Z',
  }));
  const before = structuredClone(campaigns);
  const ids = (items) => items.map((item) => item.id);
  assert.deepEqual(ids(filterAt(campaigns, '  SUMMER  ')), ['3']);
  assert.deepEqual(ids(filterAt(campaigns, '游戏首页')), ['1']);
  assert.deepEqual(ids(filterAt(campaigns, '', 'feed-recommendation')), [
    '2',
    '3',
  ]);
  assert.deepEqual(
    ids(filterAt(campaigns, '测试', 'feed-recommendation', 'ACTIVE')),
    ['2'],
  );
  assert.deepEqual(filterAt(campaigns, '', 'game-home-banner', 'ACTIVE'), []);
  assert.deepEqual(ids(filterAt(campaigns, '', allCampaignFilters, 'ENDED')), [
    '4',
  ]);
  assert.deepEqual(filterAt(campaigns, ''), campaigns);
  assert.deepEqual(filterAt([], ''), []);
  assert.deepEqual(campaigns, before);
});
import {
  adSlotOptions,
  defaultAdSlotID,
  adSlotLabel,
} from '../lib/ad-slots.ts';

test('five fixed ad slots retain the original default and legacy labels', () => {
  assert.equal(adSlotOptions.length, 5);
  assert.equal(new Set(adSlotOptions.map((slot) => slot.value)).size, 5);
  assert.equal(new Set(adSlotOptions.map((slot) => slot.label)).size, 5);
  assert.equal(defaultAdSlotID, 'game-home-banner');
  for (const slot of adSlotOptions) {
    assert.match(slot.value, /^[a-z][a-z0-9-]{1,63}$/);
    assert.equal(adSlotLabel(slot.value), slot.label);
  }
  assert.equal(adSlotLabel(defaultAdSlotID), '游戏首页横幅');
  assert.equal(adSlotLabel('legacy-slot'), 'legacy-slot');
});

test('campaign, decision and simulation selectors share fixed slots without campaign-derived options', () => {
  const consoleSource = readFileSync(
    new URL('../components/adflow-console.tsx', import.meta.url),
    'utf8',
  );
  const simulationSource = readFileSync(
    new URL('../components/user-pool-simulation.tsx', import.meta.url),
    'utf8',
  );
  assert.equal(consoleSource.match(/options=\{adSlotOptions\}/g)?.length, 2);
  assert.equal(simulationSource.match(/options=\{adSlotOptions\}/g)?.length, 1);
  assert.doesNotMatch(simulationSource, /new Set\(campaigns\.map/);
  assert.doesNotMatch(simulationSource, /slots\.length > 0/);
});

test('all console forms explicitly declare a submit button', () => {
  const source = ts.createSourceFile(
    'adflow-console.tsx',
    [
      'adflow-console.tsx',
      'campaign-rule-dialog.tsx',
      'profile-workspace.tsx',
      'user-pool-simulation.tsx',
      'agent-workspace.tsx',
      'auth-gate.tsx',
    ]
      .map((file) =>
        readFileSync(new URL(`../components/${file}`, import.meta.url), 'utf8'),
      )
      .join('\n'),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const forms = [];
  function visit(node) {
    if (
      ts.isJsxElement(node) &&
      node.openingElement.tagName.getText(source) === 'form'
    ) {
      forms.push(node);
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.ok(
    forms.length >= 8,
    'expected the console forms and campaign rule editor',
  );
  for (const form of forms) {
    let submitButtons = 0;
    function findSubmit(node) {
      const element = ts.isJsxElement(node) ? node.openingElement : node;
      if (
        ts.isJsxOpeningElement(element) ||
        ts.isJsxSelfClosingElement(element)
      ) {
        if (['Button', 'button'].includes(element.tagName.getText(source))) {
          const type = element.attributes.properties.find(
            (prop) =>
              ts.isJsxAttribute(prop) && prop.name.getText(source) === 'type',
          );
          if (
            type?.initializer &&
            ts.isStringLiteral(type.initializer) &&
            type.initializer.text === 'submit'
          ) {
            submitButtons += 1;
          }
        }
      }
      // Visit children, not the opening element again, to avoid double counting.
      if (ts.isJsxElement(node)) node.children.forEach(findSubmit);
      else ts.forEachChild(node, findSubmit);
    }
    form.children.forEach(findSubmit);
    const line = source.getLineAndCharacterOfPosition(form.pos).line + 1;
    assert.equal(
      submitButtons,
      1,
      `form near line ${line} needs one explicit submit button (Base UI defaults to type=button)`,
    );
  }
});
