const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {chromium}=require('playwright');
const root=path.resolve(__dirname,'..');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try {
  const page=await browser.newPage();
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.setContent('<div class="functions-container"></div>');
  await page.addScriptTag({content:fs.readFileSync(path.join(__dirname,'mock-kano.js'),'utf8')});
  await page.evaluate(()=>{KTS_MOCK.installed=true;KTS_MOCK.profile={protocol:'wireguard',text:'[Interface]\nPrivateKey = <fixture>',networkMode:'gateway',lanInterface:'br0',ipv6Mode:'block',sources:[]};});
  const source='const KTS_STYLE="", KTS_RESOURCE={};\n'+['host.js','plugin.js'].map(p=>fs.readFileSync(path.join(root,'src',p),'utf8')).join('\n');
  const mount=()=>page.addScriptTag({content:'(()=>{'+source+'})();'});
  await mount();await page.getByText('已与设备同步',{exact:true}).waitFor();
  assert.equal(await page.locator('#network-mode').inputValue(),'gateway');
  assert.match(await page.locator('#config').inputValue(),/fixture/);
  await page.locator('#config').fill('unsaved draft');
  assert.equal(await page.evaluate(()=>!window.dispatchEvent(new Event('beforeunload',{cancelable:true}))),true);
  // Recreate the plugin with the same persistent device, as a page reload does.
  await page.locator('#kano-tunnel-studio').evaluate(el=>el.remove());
  await mount();await page.getByText('已与设备同步',{exact:true}).waitFor();
  assert.match(await page.locator('#config').inputValue(),/fixture/);
  assert.equal(await page.evaluate(()=>!window.dispatchEvent(new Event('beforeunload',{cancelable:true}))),false);
  assert.deepEqual(errors,[]);
  console.log('Saved configuration reload and unsaved navigation warning passed');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
