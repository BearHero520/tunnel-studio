const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {chromium}=require('playwright');
const root=path.resolve(__dirname,'..');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  for(const width of [1440,390]){
   const page=await browser.newPage({viewport:{width,height:1000}});
   const errors=[];page.on('pageerror',e=>errors.push(e.message));
   // Match KANO's nested menu and height clipping, which the flat mock missed.
   await page.setContent(`<!doctype html><style>
    body{margin:16px}.container{display:flex;flex-direction:column}
    .kano_function_main{width:100%}.collapse{overflow:hidden;height:0}
    .collapse_box{padding:8px}button{padding:10px}
   </style><div class="container"><div class="kano_function_main func_list_container">
    <button id="toggle">功能列表</button>
    <div class="functions-container actions collapse collapse_menu" data-name="close">
     <div><div class="collapse_box actions-buttons"><button id="existing">宿主按钮</button></div></div>
    </div></div><div id="following">原有状态区</div></div>`);
   await page.evaluate(()=>{
    const menu=document.querySelector('.collapse_menu'),box=menu.querySelector('.collapse_box');
    const resize=()=>menu.style.height=menu.dataset.name==='open'?box.getBoundingClientRect().height+'px':'0px';
    new ResizeObserver(resize).observe(box);
    document.querySelector('#toggle').onclick=()=>{menu.dataset.name=menu.dataset.name==='open'?'close':'open';resize()};
    document.querySelector('#existing').onclick=()=>document.querySelector('#existing').textContent='可用';
   });
   await page.addScriptTag({path:path.join(__dirname,'mock-kano.js')});
   const content=fs.readFileSync(process.env.KTS_LAYOUT_SCRIPT||path.join(root,'dist/tunnel-studio.js'),'utf8');
   // Use the host's HTML extraction, not addScriptTag on the plugin file.
   await page.evaluate(source=>{
    const parsed=new DOMParser().parseFromString(source,'text/html');
    for(const el of parsed.querySelectorAll('script')){
     const script=document.createElement('script');script.textContent=el.textContent;document.head.append(script);
    }
   },content);
   await page.locator('#kano-tunnel-studio').waitFor();
   assert.equal(await page.locator('.collapse #kano-tunnel-studio').count(),0,'console is clipped by host collapse container');
   assert.equal(await page.locator('.kano_function_main + #kano-tunnel-studio').count(),1);
   await page.getByRole('button',{name:'只读诊断',exact:true}).click({timeout:3000});
   await page.locator('#toggle').click();await page.locator('#existing').click();
   assert.equal(await page.locator('#existing').textContent(),'可用');
   await page.locator('#toggle').click();
   await page.getByRole('button',{name:'连接概览',exact:true}).click({timeout:3000});
   assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'layout overflows');
   assert.deepEqual(errors,[]);
   await page.screenshot({path:path.join(root,'docs',width===390?'host-phone.png':'host-desktop.png'),fullPage:true});
   await page.close();
  }
  console.log('PASS: KANO collapsed/open menu; visible interactive console; desktop/mobile; host controls');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
