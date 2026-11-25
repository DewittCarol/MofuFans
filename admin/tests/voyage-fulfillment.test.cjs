const test=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');const ts=require('typescript');
const mod={exports:{}};vm.runInNewContext(ts.transpileModule(fs.readFileSync('src/lib/voyage-fulfillment.ts','utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,{exports:mod.exports,module:mod,require});
const {parcelState,uniqueAddress,parcelChange,parcelMatchesTab}=mod.exports;
const address={name:'用户',phone:'13800000000',address:'测试地址',note:''};const parcel={id:1,version:1,uid:'10001',name:'测试用户',choices:[],items:[{label:'9月舰长礼物'}],courier:'顺丰',tracking:'SF123456',note:''};
test('readiness uses an unambiguous address but preserves conflicts and snapshot updates',()=>{
 assert.equal(parcelState(parcel),'missing');assert.equal(parcelState({...parcel,choices:[{address}]}),'ready');assert.equal(parcelState({...parcel,address}),'ready');assert.equal(parcelState({...parcel,address,tracking:''}),'packing');assert.equal(parcelState({...parcel,address,address_changed:true}),'changed');assert.equal(parcelState({...parcel,address,address_changed:true,shipped_at:'today'}),'shipped');assert.equal(parcelState({...parcel,cancelled_at:'today'}),'cancelled');assert.equal(parcelState({...parcel,block_reason:'范围已变化'}),'blocked');
 assert.equal(parcelState({...parcel,choices:[{address},{address:{...address,address:'不同地址'}}]}),'confirm');
});
test('only identical available addresses can be selected automatically',()=>{
 assert.equal(uniqueAddress(parcel),undefined);const a={recipient_id:1,version:2,address};assert.equal(uniqueAddress({...parcel,choices:[a,{...a,recipient_id:2},{address:null}]}),a);assert.equal(uniqueAddress({...parcel,choices:[a,{...a,address:{...address,address:'另一个地址'}}]}),undefined);
});
test('late address carries the displayed version; snapshots and conflicts are never replaced',()=>{
 const a={recipient_id:7,version:3,address};const p={...parcel,choices:[a]};
 assert.equal(parcelChange(p).address_recipient_id,7);assert.equal(parcelChange(p).address_version,3);
 assert.equal(parcelChange({...p,address,address_changed:true}).address_recipient_id,undefined);
 assert.equal(parcelChange({...p,choices:[a,{...a,address:{...address,address:'另一地址'}}]}).address_recipient_id,undefined);
});
test('pending keeps missing and changed addresses visible; cancelled records stay in all',()=>{
 for(const p of [parcel,{...parcel,address,address_changed:true},{...parcel,address}])assert.equal(parcelMatchesTab(p,'pending'),true);
 assert.equal(parcelMatchesTab({...parcel,shipped_at:'today'},'pending'),false);
 assert.equal(parcelMatchesTab({...parcel,shipped_at:'today'},'shipped'),true);
 assert.equal(parcelMatchesTab({...parcel,cancelled_at:'today'},'pending'),false);
 assert.equal(parcelMatchesTab({...parcel,cancelled_at:'today'},'all'),true);
});
