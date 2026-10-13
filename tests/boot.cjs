const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage();
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.setContent('<!doctype html><div class="functions-container"></div>');
  await page.addScriptTag({path:path.join(__dirname,'mock-kano.js')});
  await page.evaluate(()=>{KTS_MOCK.installed=true;KTS_MOCK.boot=true;});
  await page.addScriptTag({path:path.join(__dirname,'../dist/tunnel-studio.js')});
  await page.getByText('设备尚无已保存配置，请导入后保存。',{exact:true}).waitFor();
  assert.equal(await page.locator('#boot-state').textContent(),'已开启');
  assert.match(await page.locator('#boot-detail').textContent(),/尚无执行记录/);
  for(const [state,label] of [['waiting','等待就绪'],['started','自启完成'],['failed','自启失败'],['timeout','自启超时']]){
   await page.evaluate(state=>{KTS_MOCK.bootInfo={enabled:true,kano:true,currentBoot:true,last:{state,detail:'启动原因 <img src=x onerror=alert(1)>'}}},state);
   await page.locator('[data-action="refresh"]').click();
   await page.getByText('设备状态已刷新。',{exact:true}).waitFor();
   assert.equal(await page.locator('#boot-state').textContent(),label);
   assert.match(await page.locator('#boot-detail').textContent(),/^本次开机/);
   assert.equal(await page.locator('#boot-detail img').count(),0);
  }
  await page.evaluate(()=>KTS_MOCK.bootInfo.currentBoot=false);
  await page.locator('[data-action="refresh"]').click();
  await page.getByText('设备状态已刷新。',{exact:true}).waitFor();
  assert.equal(await page.locator('#boot-state').textContent(),'已开启');
  assert.match(await page.locator('#boot-detail').textContent(),/^上次开机/);
  await page.locator('[data-action="boot-off"]').click();
  await page.getByText('已关闭本插件开机自启。',{exact:true}).waitFor();
  assert.equal(await page.locator('#boot-state').textContent(),'未开启');
  assert.equal(await page.locator('#boot-detail').isHidden(),true);
  assert.deepEqual(errors,[]);
  console.log('PASS: boot registration vs execution, wait/failure/timeout, previous boot, disable and safe error rendering');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
