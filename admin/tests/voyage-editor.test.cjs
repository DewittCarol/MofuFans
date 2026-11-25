const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('../node_modules/typescript');
const source = fs.readFileSync(path.join(__dirname, '../src/lib/voyage-editor.ts'), 'utf8');
const moduleObject = { exports: {} };
vm.runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, { exports: moduleObject.exports, module: moduleObject, structuredClone });
const { validateVoyageIssue, createVoyageEditorDraft, collectVoyageImages, patchVoyageDraft, reuseVoyageImage, freshVoyageCatalog } = moduleObject.exports;
const gift = (values = {}) => ({ name: '纪念徽章', section: 'guard', tiers: ['captain'], description: '', images: [], ...values });
const issue = (values = {}) => ({ id: 1, kind: 'guard', title: '九月舰礼', month: '2026-09', note: '', published: false, version: 3, gifts: [], ...values });

test('a hidden period title takes priority and identifies the settings input', () => {
  const result = validateVoyageIssue(issue({ title: '  ', published: true, gifts: [gift({ name: '' })] }));
  assert.equal(result.index, null);
  assert.equal(result.field, 'voyage-title');
});

test('an empty draft can be saved; publishing it reveals the add-gift action', () => {
  assert.equal(validateVoyageIssue(issue()), null);
  const result = validateVoyageIssue(issue({ published: true }));
  assert.equal(result.field, 'add-voyage-gift');
});

test('saving identifies the first invalid collapsed gift and its exact input', () => {
  const draft = issue({ gifts: [gift(), gift({ name: '' }), gift({ tiers: [] })] });
  let result = validateVoyageIssue(draft);
  assert.equal(result.index, 1);
  assert.equal(result.field, 'gift-name-1');
  draft.gifts[1].name = '立牌';
  result = validateVoyageIssue(draft);
  assert.equal(result.index, 2);
  assert.equal(result.field, 'gift-name-2');
});

test('monthly categories have separate validation and drafts', () => {
  const lottery = freshVoyageCatalog('2026-08', 'lottery');
  assert.equal(lottery.kind, 'lottery');
  assert.equal(lottery.month, '2026-08');
  assert.equal(validateVoyageIssue(lottery).field, 'voyage-title');
  lottery.title = '转盘活动'; lottery.gifts = [gift({section: 'other', tiers: [], audience: '中奖观众'})];
  const before = structuredClone(lottery);
  assert.equal(validateVoyageIssue(lottery), null);
  assert.deepEqual(JSON.parse(JSON.stringify(lottery)), before);
  lottery.gifts[0].tiers = ['captain'];
  assert.match(validateVoyageIssue(lottery).message, /不关联/);
  for (const month of ['', '2026-13', '0000-01']) assert.equal(validateVoyageIssue(issue({month})).field, 'voyage-month');
  assert.equal(validateVoyageIssue(freshVoyageCatalog('2026-09', 'guard')), null);
});

test('opening a card preserves explicit poster and gift selections without changing saved data', () => {
  const saved = issue({ images: ['/overview.png', '/gift.png'], gifts: [gift({ images: ['/gift.png'] })] });
  const before = structuredClone(saved);
  const draft = createVoyageEditorDraft(saved);
  assert.deepEqual([...draft.images], ['/overview.png', '/gift.png']);
  assert.deepEqual([...draft.gifts[0].images], ['/gift.png']);
  assert.deepEqual([...collectVoyageImages(saved)], ['/overview.png', '/gift.png']);
  draft.gifts = [];
  assert.deepEqual([...draft.images], ['/overview.png', '/gift.png'], 'removing a gift must not alter poster selections');
  assert.deepEqual(saved, before);
});

test('old multi-tier gifts become independent editable items, including independent image arrays', () => {
  const saved = issue({ gifts: [gift({ tiers: ['captain', 'admiral', 'governor'], images: ['/same.png'], shipping_note: '九月寄出' })] });
  const draft = createVoyageEditorDraft(saved);
  assert.equal(draft.gifts.length, 3);
  assert.deepEqual(draft.gifts.map(gift => [...gift.tiers]), [['captain'], ['admiral'], ['governor']]);
  draft.gifts[0].images.push('/captain.png');
  draft.gifts[0].name = '舰长的新礼物';
  draft.gifts[0].shipping_note = '十月寄出';
  assert.deepEqual([...draft.gifts[1].images], ['/same.png']);
  assert.deepEqual([...draft.gifts[2].images], ['/same.png']);
  assert.equal(draft.gifts[1].name, saved.gifts[0].name);
  assert.equal(draft.gifts[2].shipping_note, '九月寄出');
  assert.deepEqual(saved.gifts[0].images, ['/same.png']);
  const reopened = createVoyageEditorDraft(JSON.parse(JSON.stringify(draft)));
  assert.equal(reopened.gifts.length, 3);
  assert.equal(validateVoyageIssue(reopened), null);
  assert.deepEqual([...reopened.gifts[0].images], ['/same.png', '/captain.png']);
});

test('images can be reused from poster to gifts and back without moving any existing placement', () => {
  const draft = createVoyageEditorDraft(issue({ images: ['/overview.png'], gifts: [gift(), gift({ tiers: ['admiral'], images: ['/second.png'] })] }));
  const copied = reuseVoyageImage(draft, 0, '/overview.png');
  assert.deepEqual([...copied.images], ['/overview.png']);
  assert.deepEqual([...copied.gifts[0].images], ['/overview.png']);
  assert.deepEqual([...copied.gifts[1].images], ['/second.png']);
  assert.deepEqual([...draft.gifts[0].images], [], 'selection changes the draft immutably');
  const reused = reuseVoyageImage(copied, 0, '/second.png');
  const poster = reuseVoyageImage(reused, null, '/second.png');
  assert.deepEqual([...poster.images], ['/overview.png', '/second.png']);
  assert.deepEqual([...poster.gifts[0].images], ['/overview.png', '/second.png']);
  assert.deepEqual([...poster.gifts[1].images], ['/second.png']);
  assert.equal(reuseVoyageImage(poster, 0, '/second.png'), poster, 'duplicate selection is a no-op');
  assert.equal(reuseVoyageImage(poster, null, '/second.png'), poster);
  const reopened = createVoyageEditorDraft(JSON.parse(JSON.stringify(poster)));
  assert.deepEqual([...reopened.images], ['/overview.png', '/second.png']);
  assert.deepEqual([...reopened.gifts[0].images], ['/overview.png', '/second.png']);
  const full = issue({ gifts: [gift({ images: Array.from({ length: 6 }, (_, i) => `/${i}.png`) })] });
  assert.throws(() => reuseVoyageImage(full, 0, '/extra.png'), /最多 6 张/);
  assert.throws(() => reuseVoyageImage(issue({images: Array.from({length: 24}, (_, i) => `/${i}.png`)}), null, '/extra.png'), /最多 24 张/);
});


test('each card retains its own images after all placements are removed and saved again',()=>{
 const first=createVoyageEditorDraft(issue({images:['/poster.png'],gifts:[gift({images:['/gift.png']})]}));
 const second=createVoyageEditorDraft(issue({id:2,month:'2026-10',images:['/other-month.png']}));
 const removed=patchVoyageDraft(first,{images:[],gifts:[]});
 assert.deepEqual([...collectVoyageImages(removed)],['/poster.png','/gift.png']);
 const reopened=createVoyageEditorDraft(JSON.parse(JSON.stringify(removed)));
 assert.deepEqual([...collectVoyageImages(reopened)],['/poster.png','/gift.png']);
 assert.deepEqual([...collectVoyageImages(second)],['/other-month.png']);
 const back=reuseVoyageImage(reopened,null,'/gift.png');
 assert.deepEqual([...back.images],['/gift.png']);
 const uploaded=patchVoyageDraft(back,{images:['/gift.png','/new.png']});
 assert.deepEqual([...collectVoyageImages(uploaded)],['/poster.png','/gift.png','/new.png']);
 assert.deepEqual([...collectVoyageImages(first)],['/poster.png','/gift.png']);
});
