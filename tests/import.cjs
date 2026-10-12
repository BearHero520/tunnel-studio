const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {execFileSync}=require('node:child_process');
const {chromium}=require('playwright');
const root=path.resolve(__dirname,'..');
const fixed=fs.readFileSync(path.join(root,'dist/tunnel-studio.js'),'utf8');
const legacy=execFileSync('git',['show','v0.1.0:dist/tunnel-studio.js'],{cwd:root,maxBuffer:8*1024*1024,encoding:'utf8'});
// KANO main.js customHead contract: DOMParser, clone styles, recreate scripts.
async function importPlugin(page,source){
 return page.evaluate(head_text=>{
  const doc=new DOMParser().parseFromString(head_text,'text/html');
  doc.querySelectorAll('style, link, meta').forEach(el=>document.head.appendChild(el.cloneNode(true)));
  doc.querySelectorAll('script').forEach(el=>{
   const script=document.createElement('script');
   if(el.src)script.src=el.src;else script.textContent=el.textContent;
   if(el.type)script.type=el.type;
   document.head.appendChild(script);
  });
  return doc.querySelectorAll('script').length;
 },'<!-- [KANO_PLUGIN_START] tunnel-studio.js -->\n'+source+'\n<!-- [KANO_PLUGIN_END] tunnel-studio.js -->');
}
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage();
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/*',r=>r.abort());
  await page.setContent('<!doctype html><div class="functions-container"></div><button id="existing" onclick="this.textContent=\'works\'">host</button>');
  await page.addScriptTag({path:path.join(__dirname,'mock-kano.js')});
  assert.equal(await importPlugin(page,legacy),0,'published regression must reproduce');
  assert.equal(await page.locator('#kano-tunnel-studio').count(),0);
  assert.equal(await importPlugin(page,fixed),1);
  await page.locator('#kano-tunnel-studio').waitFor();
  await page.locator('#existing').click();assert.equal(await page.locator('#existing').textContent(),'works');
  await importPlugin(page,fixed);assert.equal(await page.locator('#kano-tunnel-studio').count(),1);
  assert.ok((await page.locator('body').textContent()).length<50000,'no embedded payload rendered as body text');
  await page.getByRole('button',{name:'只读诊断',exact:true}).click();
  assert.deepEqual(errors,[]);
  console.log('PASS: legacy import reproduced; fixed KANO HTML extraction, entry, host interaction and duplicate import');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
