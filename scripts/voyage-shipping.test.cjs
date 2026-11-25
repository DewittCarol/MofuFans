const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const ts = require('../admin/node_modules/typescript');
const delivery = {exports:{}};
vm.runInNewContext(ts.transpileModule(fs.readFileSync(`${__dirname}/../admin/src/lib/voyage-delivery.ts`,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText,delivery);
const source = fs.readFileSync(`${__dirname}/../admin/src/lib/voyage-recipients.ts`, 'utf8');
const context = {exports:{},require:(name)=>name==='./voyage-delivery'?delivery.exports:({voyageTiers:[{id:'captain',name:'舰长'}]})};
vm.runInNewContext(ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText,context);
const {recipientMessage} = context.exports;
const recipient={issue_id:8,uid:'10001',name:'离线测试',tier:'captain',code:'aB3dE6'};

test('no-tier recipients omit the default tier line and retain other template spacing',()=>{
 const text=recipientMessage({template:'你好，{昵称}！\r\n本期档位：{档位}\r\n\r\n{UID} {认证码}\r\n{登记地址}',entry_url:''},{id:8,title:'8月'},{...recipient,tier:''});
 assert.equal(text,'你好，离线测试！\n\n10001 aB3dE6\n本期登记入口：/voyage/collections/?batch=8');
});

test("streamer name follows the current site",()=>{assert.equal(recipientMessage({template:"感谢你对{主播}的支持",entry_url:""},{id:1,title:"本月"},recipient,"当前主播"),"感谢你对当前主播的支持");});
