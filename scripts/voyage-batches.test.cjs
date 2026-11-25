const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const ts=require('../admin/node_modules/typescript');
const delivery = {exports:{}};
vm.runInNewContext(ts.transpileModule(fs.readFileSync(`${__dirname}/../admin/src/lib/voyage-delivery.ts`,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText,delivery);
const source=fs.readFileSync(`${__dirname}/../admin/src/lib/voyage-recipients.ts`,'utf8');
const context={exports:{},require:(name)=>name==='./voyage-delivery'?delivery.exports:({voyageTiers:[{id:'captain',name:'舰长'},{id:'admiral',name:'提督'},{id:'governor',name:'总督'}]})};
vm.runInNewContext(ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText,context);
const {recipientMessage}=context.exports;
test('batch message uses only the current recipient snapshot, not global month scope',()=>{
 const issue={id:9,title:'9月集中寄送',batch:true};
 const recipient={uid:'12345',name:'测试',tier:'',code:'aB3dE6',batch_items:[{label:'2026年5月舰长礼物'},{label:'2026年7月总督礼物'},{label:'8月转盘抽奖'}]};
 const text=recipientMessage({template:'{批次名称}\n{本次礼物}\n{UID} {认证码}\n{登记地址}',entry_url:''},issue,recipient);
 assert.match(text,/2026年5月舰长礼物、2026年7月总督礼物、8月转盘抽奖/);
 assert.doesNotMatch(text,/6月/);assert.match(text,/voyage\/collections\/\?batch=9/);assert.match(text,/12345 aB3dE6/);
 const other=recipientMessage({template:'{本次礼物}',entry_url:''},issue,{...recipient,batch_items:[{label:'2026年6月提督礼物'}]});
 assert.equal(other,'2026年6月提督礼物');
});
test('personal scope placeholder respects a custom registration URL',()=>{
 const text=recipientMessage({template:'{本次期次}\n{登记地址}',entry_url:'https://example.com/custom'},{id:8,title:'收集批次',batch:true},{uid:'12345',code:'ABC123',batch_items:[{label:'5月舰长礼物'}]});
 assert.equal(text,'5月舰长礼物\n本期登记入口：https://example.com/custom');
});
