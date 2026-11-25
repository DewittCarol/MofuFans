const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');

function evaluate(source, globals = {}) {
  const mod = {exports: {}};
  vm.runInNewContext(ts.transpileModule(source, {compilerOptions: {module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022}}).outputText, {exports: mod.exports, module: mod, ...globals});
  return mod.exports;
}
const {SiteDialogStore} = evaluate(fs.readFileSync('src/lib/site-dialog.ts', 'utf8'));
const {guardNavigation, isPlainNavigation} = evaluate(fs.readFileSync('src/lib/guarded-navigation.ts', 'utf8'));
const tick = () => new Promise(resolve => setImmediate(resolve));
const options = {title: '删除记录？', confirmLabel: '删除记录', destructive: true};

function event(extra = {}) {
  return {defaultPrevented: false, stopped: false, button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false,
    preventDefault() { this.defaultPrevented = true; }, stopPropagation() { this.stopped = true; }, ...extra};
}

test('confirmations wait for a choice; cancel never approves and double clicks do not queue deletion', async () => {
  const store = new SiteDialogStore();
  let changed = 0, result;
  store.subscribe(() => changed++);
  const first = store.open('删除 UID 10001 的舰长记录', options).then(value => { result = value; return value; });
  await tick();
  assert.equal(result, undefined);
  assert.equal(await store.open('another deletion', options), false);
  store.finish(store.getSnapshot().id, false);
  assert.equal(await first, false);
  assert.equal(store.getSnapshot(), null);
  assert.equal(changed, 2);
});

test('only the current dialog can approve; old close events cannot dismiss a newer one', async () => {
  const store = new SiteDialogStore();
  const first = store.open('first', options), old = store.getSnapshot().id;
  store.finish(old, true);
  assert.equal(await first, true);
  const second = store.open('second', options), current = store.getSnapshot().id;
  store.finish(old, false);
  assert.equal(store.getSnapshot().id, current);
  store.cancel();
  assert.equal(await second, false);
});

test('background errors are not lost; navigating away cancels all pending dialogs', async () => {
  const store = new SiteDialogStore();
  const confirm = store.open('delete', options);
  const message = store.open('network failed', {title: '操作失败'}, 'message');
  store.finish(store.getSnapshot().id, false);
  assert.equal(await confirm, false);
  assert.equal(store.getSnapshot().description, 'network failed');
  const another = store.open('another error', {title: '操作失败'}, 'message');
  store.cancel();
  assert.equal(await message, false);
  assert.equal(await another, false);
  assert.equal(store.getSnapshot(), null);
});

test('link navigation is blocked synchronously and resumes exactly once after confirmation', async () => {
  const store = new SiteDialogStore();
  let navigations = 0;
  const click = event();
  guardNavigation(click, () => store.open('leave draft?', options), () => navigations++);
  assert.equal(click.defaultPrevented, true);
  assert.equal(click.stopped, true);
  assert.equal(navigations, 0);
  store.finish(store.getSnapshot().id, true);
  await tick();
  assert.equal(navigations, 1);
});

test('cancelled or failed link confirmations keep the current page; existing synchronous guards still work', async () => {
  let navigations = 0;
  for (const choice of [false, Promise.resolve(false), Promise.reject(Error('dialog unavailable'))]) {
    const click = event();
    guardNavigation(click, () => choice, () => navigations++);
    assert.equal(click.defaultPrevented, true);
  }
  await tick();
  assert.equal(navigations, 0);
  const click = event();
  guardNavigation(click, () => true, () => navigations++);
  assert.equal(click.defaultPrevented, false);
  assert.equal(navigations, 0); // The original link handles ordinary navigation.
  for (const extra of [{ctrlKey: true}, {metaKey: true}, {shiftKey: true}, {altKey: true}, {button: 1}, {defaultPrevented: true}]) assert.equal(isPlainNavigation(event(extra)), false);
});

function rosterRemove(globals) {
  const source = ts.createSourceFile('voyage-rosters.tsx', fs.readFileSync('src/components/voyage-rosters.tsx', 'utf8'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let initializer;
  function visit(node) {
    if (ts.isVariableDeclaration(node) && node.name.getText(source) === 'remove') initializer = node.initializer.getText(source);
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.ok(initializer);
  return evaluate(`exports.remove = ${initializer}`, globals).remove;
}

test('actual roster deletion handler submits only the confirmed record IDs and versions', async () => {
  const store = new SiteDialogStore(), requests = [];
  let rows = [{id: 1, uid: '10001', tier: 'captain', version: 2}, {id: 2, uid: '10001', tier: 'admiral', version: 3}];
  const remove = rosterRemove({busy: false, editing: null, roster: {category: 'monthly', version: 4}, voyageTiers: [{id: 'captain', name: '舰长'}], path: '/rosters/1/members', confirmAction: (...args) => store.open(...args),
    run: fn => fn(), voyageRequest: async (...args) => { requests.push(args); return {deleted: args[2].items.length, sync: {}}; },
    setRows: fn => { rows = fn(rows); }, setSelectedIDs() {}, setPreview() {}, setNotice() {}, syncNotice: () => '', onChange: async () => {}});
  const cancelled = remove([rows[0]]);
  assert.equal(requests.length, 0);
  store.cancel(); await cancelled; await tick();
  assert.equal(requests.length, 0); assert.equal(rows.length, 2);
  const approved = remove([...rows]);
  assert.equal(requests.length, 0);
  store.finish(store.getSnapshot().id, true); await approved; await tick();
  assert.equal(requests.length, 1);
  assert.equal(JSON.stringify(requests[0][2]), JSON.stringify({version: 4, items: [{id: 1, version: 2}, {id: 2, version: 3}]}));
  assert.equal(rows.length, 0);
});

test('both apps use the same dialog contract and ship no browser alert/confirm/prompt calls', () => {
  for (const rel of ['lib/site-dialog.ts', 'components/site-dialog-host.tsx']) assert.equal(fs.readFileSync('src/' + rel, 'utf8'), fs.readFileSync('../frontend/src/' + rel, 'utf8'));
  function scan(directory) {
    for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
      const file = path.join(directory, entry.name);
      if (entry.isDirectory()) { scan(file); continue; }
      if (!/\.(tsx?|jsx?)$/.test(file)) continue;
      const source = ts.createSourceFile(file, fs.readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true);
      function visit(node) {
        if (ts.isCallExpression(node)) {
          const callee = node.expression.getText(source);
          assert.ok(!/^(?:(?:window|globalThis)\.)?(?:alert|confirm|prompt)$/.test(callee), `${file}: native ${callee}`);
        }
        ts.forEachChild(node, visit);
      }
      visit(source);
    }
  }
  scan('src'); scan('../frontend/src');
});
