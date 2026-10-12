const {test}=require('node:test'),assert=require('node:assert/strict');
const host=require('../src/host.js');
test('KANO exit framing refuses truncated and failed command replies',()=>{
 assert.equal(host.unpack('{"ok":true}\nKTS_1:0\n','KTS_1'),'{"ok":true}');
 assert.throws(()=>host.unpack('looks successful','KTS_1'),/没有完整结束/);
 assert.throws(()=>host.unpack('{"error":"拒绝接管"}\nKTS_1:1\n','KTS_1'),/拒绝接管/);
 assert.throws(()=>host.unpack('old\nKTS_1:0\nextra','KTS_1'));
});
test('uploaded paths stay inside the KANO app files directory',()=>{
 assert.equal(host.uploadedPath('/uploads/a.bin'),'/data/data/com.minikano.f50_sms/files/uploads/a.bin');
 assert.equal(host.uploadedPath('uploads/a.bin'),'/data/data/com.minikano.f50_sms/files/uploads/a.bin');
 for(const bad of ['//evil/a','/../a','/x/../../a','/x/./a','file:///a','/a;id','/a\nwhoami','/x/a%2fb'])assert.throws(()=>host.uploadedPath(bad));
});
test('shell quoting and error redaction do not expand secrets',()=>{
 assert.equal(host.quote("x'$(id)"),"'x'\\''$(id)'");
 assert.equal(host.sanitize('A'.repeat(43)+'='),'[密钥已隐藏]');
});
