const test = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { createHash } = require('node:crypto');
const root = resolve(__dirname, '..');
const manifest = require('../shared/joiask-emoji-assets.json');
const defaults = require('../internal/database/emoji-defaults.json');

function animationFrames(bytes, path) {
  if (path.endsWith('.png')) {
    assert.equal(bytes.subarray(0, 8).toString('hex'), '89504e470d0a1a0a');
    for (let offset = 8; offset + 12 <= bytes.length; offset += bytes.readUInt32BE(offset) + 12) {
      if (bytes.toString('ascii', offset + 4, offset + 8) === 'acTL') return bytes.readUInt32BE(offset + 8);
    }
  } else {
    assert.equal(bytes.toString('ascii', 0, 4), 'RIFF');
    assert.equal(bytes.toString('ascii', 8, 12), 'WEBP');
    let frames = 0;
    for (let offset = 12; offset + 8 <= bytes.length;) {
      const size = bytes.readUInt32LE(offset + 4);
      if (bytes.toString('ascii', offset, offset + 4) === 'ANMF') frames++;
      offset += size + 8 + (size % 2);
    }
    return frames;
  }
  return 0;
}
test('all original image assets retain upstream bytes and animation frames', () => {
  assert.equal(manifest.assets.length, 42);
  for (const asset of manifest.assets) {
    const bytes = readFileSync(resolve(root, asset.path));
    assert.equal(createHash('sha256').update(bytes).digest('hex'), asset.sha256, asset.path);
    assert.ok(animationFrames(bytes, asset.path) > 1, `${asset.path} must remain animated`);
  }
});
test('all builtin image URLs resolve to bundled originals on fresh deployments', () => {
  const paths = new Set(manifest.assets.map(asset => asset.path));
  const inputs = defaults.filter(item => item.kind === 'input' && item.url);
  const reactions = defaults.filter(item => item.kind === 'reaction' && item.url);
  assert.equal(inputs.length, 15);
  assert.equal(reactions.length, 10);
  for (const emoji of [...inputs, ...reactions]) assert.ok(paths.has('frontend/public' + emoji.url), emoji.value);
  for (const emoji of inputs) assert.ok(paths.has('admin/public' + emoji.url), emoji.value);
  assert.equal(new Set(defaults.map(item => item.kind + ':' + item.value)).size, defaults.length);
});
