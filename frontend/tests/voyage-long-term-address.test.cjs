const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),ts=require('typescript');
const mod={exports:{}};vm.runInNewContext(ts.transpileModule(fs.readFileSync('src/lib/voyage-claim.ts','utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,{module:mod,exports:mod.exports,require});
const {usesLongTermAddress,claimPayload,longTermSetDisabledReason}=mod.exports;
const saved={name:'离线测试',phone:'13800000000',address:'已保存的测试地址',note:'仅本次'};
const record={address:saved,version:4,long_term:{enabled:true,version:8,matches_address:false},can_edit_address:false};
test('matching describes the saved address, not whether the independent switch is enabled',()=>{
 assert.equal(usesLongTermAddress(null),false);
 assert.equal(usesLongTermAddress(record),false);
 assert.equal(usesLongTermAddress({...record,long_term:{...record.long_term,matches_address:true}}),true);
 assert.equal(usesLongTermAddress({...record,long_term:{enabled:false,matches_address:true}}),false);
});
test('ordinary save always keeps long-term consent; preference requests never contain a draft address',()=>{
 const draft={...saved,address:'尚未保存的另一地址'};
 const save=claimPayload(record,draft,' 12345 ','TEST01','save');
 assert.equal(save.long_term_action,'keep');assert.equal(save.address,draft);assert.equal(save.version,4);
 for(const action of ['set_long_term','cancel_long_term']){
  const body=claimPayload(record,draft,' 12345 ','TEST01',action);
  assert.equal(body.uid,'12345');assert.equal(body.action,action);assert.equal(body.long_term_version,8);
  assert.equal('address' in body,false);assert.equal('long_term_action' in body,false);
  if(action==='set_long_term')assert.equal(body.version,4);
 }
});
test('setting requires a saved address and no draft, but remains available after collection closes',()=>{
 assert.match(longTermSetDisabledReason(null,false),/先保存收货地址/);
 assert.match(longTermSetDisabledReason({...record,address:null},false),/先保存收货地址/);
 assert.match(longTermSetDisabledReason(record,true),/未保存/);
 assert.equal(longTermSetDisabledReason(record,false),'');
});
