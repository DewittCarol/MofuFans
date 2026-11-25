const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),ts=require('typescript');
const mod={exports:{}};vm.runInNewContext(ts.transpileModule(fs.readFileSync('src/lib/voyage-address-book.ts','utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,{module:mod,exports:mod.exports});
const {addressBookAction}=mod.exports;
test('administrator confirmation carries permission and collection versions without copying address data',()=>{
 const detail={long_term:{version:7},collections:[{recipient_id:42,version:9}]};
 const one=addressBookAction(detail,'confirm_once',12,42,'B站私信','2026-10-26T02:00:00Z');
 assert.equal(one.version,7);assert.equal(one.delivery_version,9);assert.equal(one.entry_id,12);assert.equal(one.recipient_id,42);assert.equal(one.confirmed,true);
 assert.equal('address' in one,false);assert.equal('uid' in one,false);
 const stop=addressBookAction(detail,'clear_default',12,0,'','');assert.equal(stop.confirmed,false);assert.equal(stop.confirmed_at,undefined);
});
