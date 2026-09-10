import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import {
  acceptsCreativeDrag,
  CREATIVE_DRAG_TYPE,
  droppedCreativeID,
} from '../lib/creative-drag.ts';

test('Creative dragging accepts only registered IDs with its own drag format', () => {
  const transfer = (id, types = [CREATIVE_DRAG_TYPE], files = 0) => ({
    types,
    files: { length: files },
    getData: (type) => (type === CREATIVE_DRAG_TYPE ? id : ''),
  });
  assert.equal(droppedCreativeID(transfer('tomori')), 'tomori');
  assert.equal(droppedCreativeID(transfer('tomori'), true), null);
  for (const id of ['', 'unknown', 'https://example.com/a.png', '../anon.webp'])
    assert.equal(droppedCreativeID(transfer(id)), null);
  assert.equal(droppedCreativeID(transfer('anon', ['text/plain'])), null);
  assert.equal(droppedCreativeID(transfer('anon', ['text/uri-list'])), null);
  assert.equal(
    droppedCreativeID(transfer('anon', [CREATIVE_DRAG_TYPE, 'Files'], 1)),
    null,
  );
  assert.equal(
    droppedCreativeID(transfer('anon', [CREATIVE_DRAG_TYPE], 1)),
    null,
  );
});

test('Drag hover does not read protected payload; failed reads cannot change selection', () => {
  const protectedTransfer = {
    types: [CREATIVE_DRAG_TYPE],
    getData: () => {
      throw new Error('protected data');
    },
  };
  assert.equal(acceptsCreativeDrag(protectedTransfer), true);
  assert.equal(droppedCreativeID(protectedTransfer), null);
});

test('Picker keeps click/keyboard alternatives, rejects navigation on drop and never submits itself', () => {
  const source = readFileSync(
    new URL('../components/creative-asset-picker.tsx', import.meta.url),
    'utf8',
  );
  assert.match(source, /draggable=\{!disabled\}/);
  assert.match(source, /setData\(CREATIVE_DRAG_TYPE, asset.id\)/);
  assert.match(
    source,
    /onDrop=\{[\s\S]*?preventDefault\(\)[\s\S]*?droppedCreativeID\(event.dataTransfer, disabled\)/,
  );
  assert.match(source, /onClick=\{\(\) => select\(asset.id\)\}/);
  assert.match(source, /onDragEnd=\{endDrag\}/);
  assert.match(source, /onBrowse\(\)/);
  assert.match(source, /window.addEventListener\('dragend', endDrag\)/);
  assert.doesNotMatch(source, /api\.|fetch\(|type="submit"|type="file"/);
});

test('Receiver and create form are on the left; the right panel contains the library only', () => {
  const source = readFileSync(
    new URL('../components/console/creatives-view.tsx', import.meta.url),
    'utf8',
  );
  const view = source;
  const left = view.slice(0, view.indexOf('<Card className="xl:sticky'));
  const right = view.slice(view.indexOf('<Card className="xl:sticky'));
  assert.match(left, /<CreativeDropZone/);
  assert.match(left, /aria-label="左侧添加素材"/);
  assert.match(left, /确认添加素材/);
  assert.match(right, /<CreativeAssetPicker/);
  assert.doesNotMatch(right, /<CreativeDropZone|<form/);
  assert.match(view, /target\?\.focus\(\)/);
});

test('Pixel sprites have bounded dimensions and explicit contain instead of fill cropping', async () => {
  const source = readFileSync(
    new URL('../components/creative-image.tsx', import.meta.url),
    'utf8',
  );
  assert.doesNotMatch(source, /\bfill\b|object-cover/);
  assert.match(source, /objectFit: 'contain'/);
  assert.match(source, /width=\{size\}/);
  assert.match(source, /height=\{size\}/);
  const { getImageProps } =
    await import('../node_modules/vinext/dist/shims/image.js');
  const { props } = getImageProps({
    src: '/demo-assets/bangdream/anon.webp',
    alt: 'test',
    width: 160,
    height: 160,
    unoptimized: true,
    style: {
      width: 160,
      height: 160,
      objectFit: 'contain',
      imageRendering: 'pixelated',
    },
  });
  assert.equal(props.width, 160);
  assert.equal(props.height, 160);
  assert.equal(props.style.objectFit, 'contain');
  assert.notEqual(props['data-nimg'], 'fill');
});
import {
  demoCreatives,
  creativeImageURL,
  isDemoCreative,
  localCreativeImageURL,
} from '../lib/demo-creatives.ts';

test('Local-only creation resolves registered asset IDs and rejects arbitrary URLs', () => {
  const asset = demoCreatives[0];
  assert.equal(
    localCreativeImageURL(asset.id, 'http://127.0.0.1:3000'),
    'http://127.0.0.1:3000' + asset.path,
  );
  for (const id of [
    '',
    'missing',
    '../anon.webp',
    'https://example.com/ad.png',
    asset.path,
  ])
    assert.throws(() => localCreativeImageURL(id, 'http://127.0.0.1:3000'));
  const source = readFileSync(
    new URL('../components/console/creatives-view.tsx', import.meta.url),
    'utf8',
  );
  const view = source;
  assert.doesNotMatch(view, /label="图片 URL"|setImageUrl/);
  assert.match(
    view,
    /localCreativeImageURL\(assetID, window.location.origin\)/,
  );
  assert.match(
    view,
    /api\.enableCreative\(\s*item\.campaignId,\s*item\.id,?\s*\)/,
  );
  assert.match(view, /item.status === 'DISABLED'/);
});
import {
  nextTestCreativeNumber,
  selectedCreativeCampaignID,
  testCreativeTitle,
} from '../lib/creative-defaults.ts';

test('Local default pictures resolve to the frontend origin for the HTTP-only backend', () => {
  const path = demoCreatives[0].path;
  assert.equal(
    creativeImageURL(path, 'http://127.0.0.1:3000'),
    'http://127.0.0.1:3000' + path,
  );
  assert.equal(
    creativeImageURL('https://example.com/ad.png', 'http://127.0.0.1:3000'),
    'https://example.com/ad.png',
  );
  assert.equal(isDemoCreative('http://localhost:3000' + path), true);
  assert.equal(isDemoCreative('https://example.com/ad.png'), false);
  for (const input of [
    '',
    'not a URL',
    'javascript:alert(1)',
    'file:///tmp/a.png',
    'data:image/png;base64,x',
  ])
    assert.throws(() => creativeImageURL(input, 'http://localhost:3000'));
});

test('Complete collection is present and preserves the original three assets', () => {
  const hashes = {
    anon: '7a4d5ccb55a7fbe89cd4c53c95b25c6d981affee70c14af5e5afaef20c88f034',
    tomori: '967f63c10a2aa32d43b84f7df11108e9704ac83eb5ef5d2b3bf2d63e8c08997c',
    rana: '32562efc8382f79513ccecd6bdb56dc7b70a495ed1db06ea77eabe3b61a6c5f3',
  };
  assert.equal(demoCreatives.length, 19);
  assert.equal(new Set(demoCreatives.map((asset) => asset.path)).size, 19);
  assert.equal(new Set(demoCreatives.map((asset) => asset.id)).size, 19);
  for (const asset of demoCreatives) {
    const buffer = readFileSync(
      new URL('../public' + asset.path, import.meta.url),
    );
    assert.equal(buffer.toString('ascii', 0, 4), 'RIFF');
    assert.equal(buffer.toString('ascii', 8, 12), 'WEBP');
    assert.equal(
      createHash('sha256').update(buffer).digest('hex'),
      asset.sha256,
    );
    assert.ok(asset.source && asset.author);
    if (hashes[asset.id]) assert.equal(asset.sha256, hashes[asset.id]);
  }
});

test('Creative defaults use current-plan gaps and ignore historical browser numbering', () => {
  assert.equal(testCreativeTitle(nextTestCreativeNumber([])), '测试素材 001');
  assert.equal(
    testCreativeTitle(nextTestCreativeNumber([], 8)),
    '测试素材 001',
  );
  assert.equal(
    testCreativeTitle(
      nextTestCreativeNumber(
        [{ title: '测试素材 009' }, { title: '自定义 100' }],
        2,
      ),
    ),
    '测试素材 001',
  );
  for (const floor of [NaN, Infinity, 0, -1, 1.5])
    assert.equal(nextTestCreativeNumber([], floor), 1);
  assert.equal(testCreativeTitle(1000), '测试素材 1000');
});

test('Successful creation clears only the pending selection and blocks empty resubmission', () => {
  const source = readFileSync(
    new URL('../components/console/creatives-view.tsx', import.meta.url),
    'utf8',
  );
  const view = source;
  assert.match(
    view,
    /await api\.createCreative[\s\S]*?setTitle\(null\);\s*setAssetID\(''\);\s*await load/,
  );
  assert.match(
    view,
    /creating.current\s*\|\|\s*busy\s*\|\|\s*!canOperate\s*\|\|\s*!selectedCampaignID\s*\|\|\s*!assetID/,
  );
  assert.match(
    view,
    /disabled=\{\s*busy\s*\|\|\s*!canOperate\s*\|\|\s*!selectedCampaignID\s*\|\|\s*!assetID\s*\|\|\s*loadingCreatives/,
  );
  assert.ok(
    view.includes(
      "key={`${selectedCampaignID}:${assetID || 'empty-selection'}`}",
    ),
  );
});

test('Creative selection starts empty and changing plans clears the pending image', () => {
  const source = readFileSync(
    new URL('../components/console/creatives-view.tsx', import.meta.url),
    'utf8',
  );
  const view = source;
  assert.match(view, /\[assetID, setAssetID\] = useState\(''\)/);
  assert.doesNotMatch(view, /useState\(demoCreatives\[0\]/);
  assert.match(view, /setCampaignID\(nextID\);\s*setAssetID\(''\)/);
});

test('Plan choice stays explicit across refresh and list responses cannot overwrite a newer choice', () => {
  const campaigns = [{ id: '14' }, { id: '13' }];
  assert.equal(selectedCreativeCampaignID(campaigns, ''), '');
  assert.equal(selectedCreativeCampaignID(campaigns, '13'), '13');
  assert.equal(
    selectedCreativeCampaignID([...campaigns].reverse(), '13'),
    '13',
  );
  assert.equal(selectedCreativeCampaignID([{ id: '14' }], '13'), '');
  const source = readFileSync(
    new URL('../components/console/creatives-view.tsx', import.meta.url),
    'utf8',
  );
  const view = source;
  assert.doesNotMatch(view, /campaignID \|\| campaigns\[0\]/);
  assert.match(view, /label="选择广告计划"/);
  assert.match(view, /request === creativeRequest.current/);
  assert.match(view, /cancelCreativeLoad\(\);\s*setCampaignID/);
});

test('Custom or deliberately blank titles win; only successful creation advances defaults', () => {
  const source = readFileSync(
    new URL('../components/console/creatives-view.tsx', import.meta.url),
    'utf8',
  );
  const view = source;
  assert.match(view, /titleOverride \?\? testCreativeTitle\(sequence\)/);
  assert.match(
    view,
    /await api\.createCreative[\s\S]*?setItems\([\s\S]*?setTitle\(null\)/,
  );
  assert.doesNotMatch(view, /setTitle\(''\)/);
  assert.doesNotMatch(view, /adflow.test-creative-sequence/);
  assert.match(
    view,
    /creating.current\s*\|\|\s*busy\s*\|\|\s*!canOperate\s*\|\|\s*!selectedCampaignID/,
  );
});
