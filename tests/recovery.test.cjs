const {test}=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto');
const sha=s=>crypto.createHash('sha256').update(s).digest('hex');
const fixture='// Tunnel Studio 0.1.0 | fixture\n(()=>{})();\n';
const source=fs.readFileSync(path.join(__dirname,'../tools/recover-v0.1.0.js'),'utf8').replace('b81f5e7d0ab0a47255337f4b31bfe6cf22da1c49df916b37f89358e39aaeb6e7',sha(fixture));
const block=s=>'<!-- [KANO_PLUGIN_START] renamed.js -->\n'+s+'\n<!-- [KANO_PLUGIN_END] renamed.js -->';
async function run(initial,opts={}){
 let stored=initial,posts=0,reads=0;const errors=[];
 await vm.runInNewContext(source,{
  common_headers:{},CryptoJS:{SHA256:sha},console:{log(){},error:s=>errors.push(s)},
  fetch:async(url,init)=>{
   if(init.method==='POST'){
    posts++;if(opts.denied)return {ok:false,status:401};
    stored=JSON.parse(init.body).text.trim();return {ok:true,json:async()=>({result:'success'})};
   }
   reads++;return {ok:true,json:async()=>({text:opts.concurrent&&reads===2?stored+'changed':stored})};
  }
 });return {stored,posts,errors};
}
test('recovery removes only matching legacy block, preserving other plugins',async()=>{
 const other='<script>window.other=1;</script>';
 const r=await run(other+'\n'+block(fixture)+'\n'+other);
 assert.equal(r.stored,other+'\n\n'+other);assert.equal(r.posts,1);assert.deepEqual(r.errors,[]);
});
test('recovery refuses changed content and unframed text',async()=>{
 for(const text of [block(fixture+'changed'),fixture]){
  const r=await run(text);assert.equal(r.posts,0);assert.equal(r.stored,text);assert.equal(r.errors.length,1);
 }
});
test('recovery refuses concurrent edits and reports authentication failure',async()=>{
 const initial=block(fixture);
 const concurrent=await run(initial,{concurrent:true});assert.equal(concurrent.posts,0);
 const denied=await run(initial,{denied:true});assert.equal(denied.stored,initial);assert.match(denied.errors[0],/401/);
});
