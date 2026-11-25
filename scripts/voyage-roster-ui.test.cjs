const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const ts=require('../admin/node_modules/typescript');
function load(path){
 const context={exports:{},require:(name)=>name==='./voyage-delivery'?load('admin/src/lib/voyage-delivery.ts'):({voyageTiers:[]})};
 vm.runInNewContext(ts.transpileModule(fs.readFileSync(`${__dirname}/../${path}`,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,jsx:ts.JsxEmit.React,target:ts.ScriptTarget.ES2020}}).outputText,context);
 return context.exports;
}
const {groupRosterMonths}=load('admin/src/components/voyage-roster-browser.tsx');
const {voyageDispatchBlockReason,recipientMessage}=load('admin/src/lib/voyage-recipients.ts');
test('month grouping retains the year, same-month events and undated activities',()=>{
 const groups=groupRosterMonths([{id:1,month:'2025-08'},{id:2,month:'2026-08'},{id:3,month:'2026-08'},{id:4,month:''}]);
 assert.equal(JSON.stringify(groups.map(g=>[g.title,g.rows.map(r=>r.id)])),JSON.stringify([['2026年8月',[2,3]],['2025年8月',[1]],['未分月份',[4]]]));
});
test('send blockers explain draft status, template edits and missing credentials; publication enables sending',()=>{
 const user={code:'ABC123',disabled:false};
 assert.match(voyageDispatchBlockReason(false,user,false,false,100),/先发布/);
 assert.match(voyageDispatchBlockReason(true,user,true,false,100),/模板/);
 assert.match(voyageDispatchBlockReason(true,{},false,false,100),/认证码/);
 assert.match(voyageDispatchBlockReason(true,user,false,false,501),/500/);
 assert.equal(voyageDispatchBlockReason(true,user,false,false,100),'');
});
test('multiple tiers in one month appear together in one recipient message',()=>{
 const text=recipientMessage({template:'{UID}：{本次礼物}',entry_url:''},{id:9,title:'9月发货',batch:true},{uid:'12345',batch_items:[{label:'2026年8月舰长礼物'},{label:'2026年8月提督礼物'}]});
 assert.equal(text,'12345：2026年8月舰长礼物、2026年8月提督礼物');
});
