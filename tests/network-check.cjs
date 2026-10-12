const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {chromium}=require('playwright');
const root=path.resolve(__dirname,'..');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try {
  const page=await browser.newPage({viewport:{width:390,height:844}});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.setContent('<div class="functions-container"></div>');
  await page.addScriptTag({content:fs.readFileSync(path.join(__dirname,'mock-kano.js'),'utf8')});
  await page.evaluate(()=>{KTS_MOCK.installed=true;KTS_MOCK.running=true;KTS_MOCK.profile={protocol:'wireguard',text:'[Interface]\nPrivateKey = <fixture>',networkMode:'gateway',sources:[]};});
  await page.addScriptTag({content:fs.readFileSync(path.join(root,'dist/tunnel-studio.js'),'utf8')});
  await page.getByText('已与设备同步',{exact:true}).waitFor();
  await page.locator('[data-tab="diagnostics"]').click();
  const check=page.locator('[data-action="network-check"]');
  await page.locator('#network-target').fill('http://192.0.2.10:8317/');
  await check.click();
  await page.getByText('联网检测完成，可导出诊断；该检测不包含已登录的 Codex 对话。',{exact:true}).waitFor();
  const calls=await page.evaluate(()=>KTS_MOCK.calls);
  assert.ok(calls.some(s=>s.replace(/'\\''/g,"'").includes("'network-check' '192.0.2.10:8317'")));
  const log=JSON.parse(await page.locator('#diagnostics').textContent());
  assert.ok(log.browserCapturedAt && log.requestDurationMs>=0 && log.samples.length===1);
  await page.screenshot({path:path.join(root,'docs/network-check-phone.png'),fullPage:true});
  const count=await page.evaluate(()=>KTS_MOCK.calls.length);
  await page.locator('#network-target').fill('http://user:secret@192.0.2.10:8317/v1');
  await check.click();
  await page.getByText('只填写内网 IPv4:端口，不包含登录信息或 API 路径。',{exact:true}).waitFor();
  assert.equal(await page.evaluate(()=>KTS_MOCK.calls.length),count);
  await page.locator('[data-action="stop"]').click();
  await page.getByText('本插件隧道已停止。',{exact:true}).waitFor();
  assert.equal(await check.isDisabled(),true);
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  assert.deepEqual(errors,[]);
  console.log('Network check input, reporting, stopped controls and mobile layout passed');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
