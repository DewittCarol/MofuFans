const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
function load(file) {
 const filename=path.join(__dirname,file); const mod={exports:{}};
 const req=name=>{
  if(name==='@/lib/voyage') return load('../src/lib/voyage.ts');
  if(name==='@/components/voyage-tier-badge') return {VoyageTierBadge:({tier})=>React.createElement('span',{'data-tier':tier},tier)};
  if(name==='@/components/ui/button') return {Button:({variant,size,...props})=>React.createElement('button',props)};
  if(name==='@/components/ui/input') return {Input:React.forwardRef((props,ref)=>React.createElement('input',{...props,ref}))};
  return require(name);
 };
 vm.runInNewContext(ts.transpileModule(fs.readFileSync(filename,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,jsx:ts.JsxEmit.ReactJSX}}).outputText,{exports:mod.exports,module:mod,require:req,structuredClone});
 return mod.exports;
}
const {VoyageCatalogCanvas}=load('../src/components/voyage-catalog-canvas.tsx');
const {createVoyageEditorDraft}=load('../src/lib/voyage-editor.ts');
const gift={name:'纪念徽章',description:'礼物说明',audience:'中奖用户',images:[],tiers:['captain','admiral']};
const render=(kind,activeField=null)=>renderToStaticMarkup(React.createElement(VoyageCatalogCanvas,{draft:createVoyageEditorDraft({id:1,kind,month:'2026-09',title:'卡片标题',note:'卡片介绍',shipping_note:'十月发出',images:[],gifts:[gift]}),activeField,busy:false,onActiveField:()=>{},onPatch:()=>{},onPatchGift:()=>{},onAddGift:()=>{},onRemoveGift:()=>{},onImages:()=>{}}));
test('the idle canvas is readable content, and a lottery has no tier controls',()=>{
 const html=render('lottery');
 assert.match(html,/卡片标题/); assert.match(html,/中奖用户/); assert.match(html,/活动奖品/);
 assert.match(html,/奖品名称/); assert.match(html,/奖品介绍/); assert.match(html,/领取说明/);
 assert.equal((html.match(/aria-label="编辑奖品"/g)||[]).length,1);
 assert.ok(html.indexOf('移除纪念徽章')<html.indexOf('aria-label="编辑奖品"'));
 assert.match(html,/移除奖品/);
 assert.doesNotMatch(html,/修改图片|添加奖品介绍/);
 assert.doesNotMatch(html,/<input|<textarea|data-tier|领取档位|基础设置|礼物清单上方|电子礼物/);
});
test('editing a tier gift exposes no tier selector and keeps other tier editors independent',()=>{
 const html=render('guard','gift-name-0');
 assert.equal((html.match(/<input/g)||[]).length,1);
 assert.equal((html.match(/<textarea/g)||[]).length,2);
 assert.match(html,/完成编辑/); assert.doesNotMatch(html,/领取档位/); assert.match(html,/移除礼物/);
 assert.doesNotMatch(html,/中奖用户/);
 const ids=[...html.matchAll(/\sid="([^"]+)"/g)].map(match=>match[1]);
 assert.equal(ids.length,new Set(ids).size);
 assert.match(html,/id="gift-name-0"/);
 assert.match(html,/id="gift-name-1"/);
});

test('overall information includes shipping instructions for both categories',()=>{
 for(const kind of ['guard','lottery']){
  const html=render(kind,'voyage-title');
  assert.match(html,/完成编辑/);assert.doesNotMatch(html,/收起编辑|编辑发货说明/);
  assert.equal((html.match(/id="voyage-shipping"/g)||[]).length,1);
  assert.ok(html.indexOf('id="voyage-shipping"')<html.indexOf(kind==='guard'?'data-tier="captain"':'活动奖品'));
  assert.match(html,kind==='guard'?/回馈图片/:/活动图片/);
 }
});
