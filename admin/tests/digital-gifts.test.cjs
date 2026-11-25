const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('../node_modules/typescript');
const source = fs.readFileSync(path.join(__dirname, '../src/lib/digital-gifts.ts'), 'utf8');
const moduleObject = { exports: {} };
vm.runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, { exports: moduleObject.exports, module: moduleObject });
const { giftMessage, giftSize } = moduleObject.exports;
test('large files have readable sizes and open gifts do not contain passwords', () => {
 assert.equal(giftSize(0), '0 B');
 assert.equal(giftSize(1024 ** 3), '1.0 GB');
 const text=giftMessage('公开礼物','','',false,true);
 assert.ok(!text.includes('礼物密码：'));
 assert.ok(text.includes('公开礼物'));
});

test('notifications contain the password but no site link; UID is required only for individual gifts', () => {
  const shared = giftMessage('九月电子礼物', 'Private12', '', false);
  const individual = giftMessage('九月电子礼物', 'Other12', '123456', true);
  assert.ok(shared.includes('礼物密码：Private12'));
  assert.ok(!shared.includes('UID：'));
  assert.ok(individual.includes('UID：123456\n礼物密码：Other12'));
  for (const text of [shared, individual]) { assert.ok(!/https?:\/\/|mofu\.fun/i.test(text)); assert.ok(text.length < 500); }
});
