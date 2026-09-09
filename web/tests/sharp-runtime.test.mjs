import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import test from 'node:test';

test('Miniflare image dependency loads its native codec and round-trips AVIF', async (t) => {
  const require = createRequire(import.meta.url);
  const fromMiniflare = createRequire(require.resolve('miniflare'));
  const sharp = fromMiniflare('sharp');
  const encoded = await sharp({
    create: { width: 8, height: 8, channels: 4, background: '#3855a7' },
  })
    .avif()
    .toBuffer();
  const decoded = await sharp(encoded)
    .ensureAlpha()
    .raw()
    .toBuffer({ resolveWithObject: true });
  assert.equal(decoded.info.width, 8);
  assert.equal(decoded.info.height, 8);
  assert.equal(decoded.info.channels, 4);
  assert.equal(decoded.data.length, 8 * 8 * 4);
  t.diagnostic(
    `sharp=${sharp.versions.sharp}; libheif=${sharp.versions.heif}; libvips=${sharp.versions.vips}`,
  );
});
