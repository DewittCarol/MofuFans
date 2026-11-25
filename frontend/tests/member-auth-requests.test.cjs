const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');

function api() {
  const filename = path.join(__dirname, '../src/lib/api.ts'), module = { exports: {} }, requests = [];
  const source = ts.transpileModule(fs.readFileSync(filename,'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText;
  vm.runInNewContext(source, { module, exports: module.exports, require, fetch: async (url, options) => { requests.push({url,...options}); return {json:async()=>({code:200,data:{}})}; } }, { filename });
  return { ...module.exports, requests };
}
test('numeric identifiers use UID by default and explicitly selected old numeric names stay distinct', async () => {
  const client = api();
  await client.loginAccount('10001','offline-password');
  await client.loginAccount('10001','offline-password',true);
  await client.loginAccount('legacy-name','offline-password');
  assert.deepEqual(JSON.parse(client.requests[0].body), {bilibili_uid:'10001',password:'offline-password'});
  assert.deepEqual(JSON.parse(client.requests[1].body), {username:'10001',password:'offline-password'});
  assert.deepEqual(JSON.parse(client.requests[2].body), {username:'legacy-name',password:'offline-password'});
});
test('logout uses JSON POST and account status cannot use a cached response', async () => {
  const client=api();await client.logoutAccount('10001');await client.getAccountInfo();
  assert.equal(client.requests[0].method,'POST'); assert.equal(client.requests[0].headers['Content-Type'],'application/json');assert.deepEqual(JSON.parse(client.requests[0].body),{uid:'10001'});
  assert.equal(client.requests[1].cache,'no-store');
});
