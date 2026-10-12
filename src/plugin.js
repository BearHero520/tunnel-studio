(async () => {
  'use strict';
  const id='kano-tunnel-studio';
  if(document.getElementById(id)) return;
  let anchor;
  for(let n=0;n<60;n++){anchor=document.querySelector('.functions-container');if(anchor)break;await new Promise(r=>setTimeout(r,100));}
  if(!anchor)return;
  const host=document.createElement('section');host.id=id;
  // KANO clips this menu to the height of .collapse_box. The console must
  // remain outside that measured region, including when the menu is closed.
  const menu=anchor.closest('.kano_function_main')||anchor;
  menu.insertAdjacentElement('afterend',host);
  host.style.width='100%';host.style.minWidth='0';
  const ui=host.attachShadow({mode:'open'});
  const style=document.createElement('style');style.textContent=KTS_STYLE;ui.append(style);
  const shell=document.createElement('div');shell.className='studio';shell.dataset.mode='auto';
  shell.innerHTML=`
  <header class="head"><div class="brand"><h1>WireGuard / AmneziaWG</h1><span class="hint">v0.1.4</span><span class="badge" id="protocol-badge">未选择协议</span></div><select class="mode" aria-label="显示模式"><option value="auto">自动布局</option><option value="desktop">桌面</option><option value="phone">手机</option></select></header>
  <div class="toolbar row"><button data-action="install">安装 / 更新</button><button class="primary" data-action="start">启动</button><button data-action="stop">停止</button><button data-action="refresh">刷新状态</button><span class="hint" id="dirty">尚未载入</span></div>
  <div class="workspace">
  <section class="editor-pane" data-panel="profile"><div class="editor-title"><strong>配置</strong><button data-action="load">读取设备配置</button></div>
    <div class="protocols"><label><input type="radio" name="protocol" value="wireguard">WireGuard</label><label><input type="radio" name="protocol" value="amneziawg" checked>AmneziaWG</label></div>
    <textarea id="config" spellcheck="false" autocomplete="off" aria-label="隧道配置" placeholder="粘贴或导入 WireGuard / AmneziaWG 配置"></textarea>
    <div class="network-row"><label for="network-mode">代理范围</label><select id="network-mode"><option value="gateway">本机 + 热点</option><option value="lan">仅热点</option><option value="device">仅本机</option><option value="tunnel">仅隧道（不代理上网）</option><option value="sources">自定义来源路由</option></select></div>
    <details><summary>网络与导入选项</summary><label for="lan-interface">热点接口</label><input id="lan-interface" value="br0" maxlength="15"><label for="ipv6-mode">IPv6</label><select id="ipv6-mode"><option value="block">阻止外网 IPv6，避免绕过 IPv4 隧道</option><option value="nat">通过隧道（需要 NAT66）</option><option value="routed">热点路由（无需 NAT66，需服务端回程路由）</option></select><label for="sources">自定义来源网段（仅自定义模式，每行一个 CIDR）</label><textarea id="sources" rows="2" spellcheck="false"></textarea><label for="import">导入 .conf / .json</label><input id="import" type="file" accept=".conf,.json,text/plain,application/json"><p class="hint">自动代理按 AllowedIPs 接管流量，并处理转发、NAT 和配置中的 DNS；旧配置不会自动改变代理范围。</p></details>
    <div class="row editor-actions"><button data-action="validate">校验</button><button data-action="save" class="primary">保存</button><button data-action="export">导出</button><button data-action="clear">清空</button></div><pre class="hint" id="validation">安装后可校验并保存配置。</pre>
  </section>
  <section class="info-pane"><nav aria-label="设备信息"><button data-tab="overview" aria-selected="true">连接概览</button><button data-tab="recovery" aria-selected="false">版本与恢复</button><button data-tab="diagnostics" aria-selected="false">只读诊断</button></nav>
  <section data-panel="overview"><div class="status"><div><span class="dot" id="dot"></span><strong id="connection">尚未读取状态</strong><div class="hint" id="status-detail">点击刷新，读取设备状态。</div></div></div>
    <div class="metrics"><div class="metric"><span class="muted">接收</span><strong id="rx">—</strong></div><div class="metric"><span class="muted">发送</span><strong id="tx">—</strong></div><div class="metric"><span class="muted">最近握手</span><strong id="handshake">—</strong></div></div>
    <div class="row"><span>开机自启</span><span class="badge" id="boot-state">未读取</span><button data-action="boot-on">开启</button><button data-action="boot-off">关闭</button></div>
    <p class="hint">握手成功不等于代理上网成功；请确认运行中的代理范围与流量计数。</p>
  </section>
  <section data-panel="recovery" hidden><h3>设备资源</h3><div class="version" id="release">未读取</div><div class="row"><button data-action="rollback">回滚上一资源版本</button></div><h3>配置备份</h3><div class="row"><button data-action="backup">创建快照</button><button data-action="backups">读取快照列表</button></div><div id="backups"><p class="hint">尚未读取快照。</p></div></section>
  <section data-panel="diagnostics" hidden><div class="row"><button data-action="preflight">检查 KANO 执行通道</button><button data-action="diagnose">生成只读诊断</button><button data-action="download-diagnostics">导出诊断</button></div><pre id="diagnostics">等待检查。</pre></section>
  </section></div><div class="message" role="status" aria-live="polite">首次使用：安装资源 → 读取或导入配置 → 选择代理范围 → 保存并启动。</div>
  <dialog><h2 id="dialog-title"></h2><p id="dialog-text"></p><div class="row"><button id="cancel">取消</button><button class="primary" id="accept">继续</button></div></dialog>`;
  ui.append(shell);
  const $=s=>ui.querySelector(s),$$=s=>[...ui.querySelectorAll(s)];
  const state={busy:false,status:null,saved:null,touched:false,diagnostics:null};
  const profile=()=>({protocol:$('input[name=protocol]:checked').value,text:$('#config').value,sources:$('#network-mode').value==='sources'?$('#sources').value.split(/\r?\n/).map(s=>s.trim()).filter(Boolean):[],networkMode:$('#network-mode').value,lanInterface:$('#lan-interface').value.trim()||'br0',ipv6Mode:$('#ipv6-mode').value});
  const dirty=()=>state.saved!==JSON.stringify(profile());
  const report=(text,error=false)=>{$('.message').textContent=KTSHost.sanitize(text);$('.message').classList.toggle('error',error);};
  function controls(){
    const s=state.status,run=Boolean(s?.running);
    $$('button[data-action]').forEach(b=>{const a=b.dataset.action;b.disabled=state.busy||(['save','restore','install','rollback','start'].includes(a)&&run)||(['save','validate','start','boot-on','boot-off','backup','backups','load','rollback','diagnose'].includes(a)&&!s?.installed)||(a==='start'&&(!s?.configured||(state.touched&&dirty())))||(a==='stop'&&!run);});
    $('#dirty').textContent=state.touched&&dirty()?'有未保存修改':state.saved===null?'尚未载入':'已与设备同步';
  }
  function fill(p){$('input[value='+p.protocol+']').checked=true;$('#config').value=p.text;$('#sources').value=(p.sources||[]).join('\n');$('#network-mode').value=p.networkMode||((p.sources||[]).length?'sources':'tunnel');$('#lan-interface').value=p.lanInterface||'br0';$('#ipv6-mode').value=p.ipv6Mode||'block';state.saved=JSON.stringify(profile());state.touched=false;controls();}
  function bytes(n){if(!Number.isFinite(n))return '—';if(n<1024)return n+' B';if(n<1048576)return (n/1024).toFixed(1)+' KiB';return (n/1048576).toFixed(1)+' MiB';}
  function paint(s){state.status=s;const running=Boolean(s.running);$('#connection').textContent=running?'隧道已启动':s.installed?'已停止':'尚未安装';$('#dot').classList.toggle('on',running);const mode=s.tunnel?.networkMode||((s.tunnel?.sources||[]).length?'sources':'tunnel');const scope={gateway:'本机 + 热点',device:'仅本机',lan:'仅热点',sources:'自定义来源路由',tunnel:'仅隧道，未接管上网流量'}[mode]||mode;$('#status-detail').textContent=running?'运行范围：'+scope+'。'+(mode==='tunnel'?'请停止后读取配置，改为本机 + 热点、保存，再启动。':'IPv6：'+(s.tunnel?.ipv6Mode||'未报告')+'；DNS 重定向：'+(s.tunnel?.dnsRedirect?'已启用':'未启用')):s.configured?'配置已保存，可以启动。':'安装资源并保存配置后即可启动。';$('#protocol-badge').textContent=s.protocol==='wireguard'?'WireGuard':s.protocol==='amneziawg'?'AmneziaWG':'未选择协议';$('#boot-state').textContent=s.boot?'已开启':'未开启';$('#release').textContent=s.release?'当前资源：'+s.release.current+'\n上一资源：'+(s.release.previous||'无'):'尚未安装';const peers=s.tunnel?.peers||[];$('#rx').textContent=running?bytes(peers.reduce((a,p)=>a+Number(p.rx_bytes||0),0)):'—';$('#tx').textContent=running?bytes(peers.reduce((a,p)=>a+Number(p.tx_bytes||0),0)):'—';const h=Math.max(0,...peers.map(p=>Number(p.last_handshake_time_sec||0)));$('#handshake').textContent=h?Math.max(0,Math.floor(Date.now()/1000-h))+' 秒前':running?'尚未握手':'—';controls();}
  async function refresh(){try{paint(await KTSHost.call('status'))}catch(e){state.status=null;$('#connection').textContent='状态未知';$('#dot').classList.remove('on');controls();throw e}}
  async function confirm(title,text){const d=$('dialog');$('#dialog-title').textContent=title;$('#dialog-text').textContent=text;return new Promise(resolve=>{const done=value=>{d.close();$('#cancel').onclick=null;$('#accept').onclick=null;d.oncancel=null;resolve(value);};$('#cancel').onclick=()=>done(false);$('#accept').onclick=()=>done(true);d.oncancel=e=>{e.preventDefault();done(false)};d.showModal();$('#cancel').focus();});}
  async function transaction(fn){if(state.busy)return;state.busy=true;controls();try{await fn();}catch(e){report(e.message||'操作未完成',true);}finally{state.busy=false;controls();}}
  function download(name,contents,type='application/json'){const url=URL.createObjectURL(new Blob([contents],{type}));const a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),30000);}
  async function listBackups(){const names=await KTSHost.call('backups');const box=$('#backups');box.replaceChildren();if(!names?.length){box.textContent='暂无快照。';return;}for(const name of names.slice().reverse()){const row=document.createElement('div');row.className='backup-item';const label=document.createElement('span');label.textContent=name;const b=document.createElement('button');b.textContent='恢复';b.disabled=Boolean(state.status?.running);b.onclick=()=>transaction(async()=>{if(!await confirm('恢复这份快照？','将替换设备配置，当前设备配置会另存快照。未保存的编辑也将被替换。'))return;await KTSHost.call('restore',name);fill(await KTSHost.call('load'));await refresh();await listBackups();report('快照已恢复，尚未启动连接。');});row.append(label,b);box.append(row);}}
  const actions={
    refresh:async()=>{await refresh();report('设备状态已刷新。');},
    preflight:async()=>{const text=await KTSHost.shell('id; uname -m; printf "Android 64-bit ABI: "; getprop ro.product.cpu.abilist64; ls -l /dev/net/tun /dev/tun 2>/dev/null; true');state.diagnostics={execution:text};$('#diagnostics').textContent=text;report('执行通道检查完成。');},
    install:async()=>{if(!await confirm('安装内置资源？','将写入 Tunnel Studio 独立目录。升级时保留配置与上一版本；不会自动启动隧道。'))return;report('正在上传并校验内置资源，请保持页面打开。');const r=await KTSHost.install(KTS_RESOURCE);if(!r.ok)throw new Error(r.error||'安装未完成');await refresh();report('资源已安装。可导入配置后校验。');},
    validate:async()=>{const r=await KTSHost.save(profile(),true);$('#validation').textContent='校验通过。\n'+(r.warnings||[]).join('\n');report('配置校验通过，尚未保存。');},
    save:async()=>{const p=profile();if(['gateway','lan','device'].includes(p.networkMode)&&state.status?.autoProxy!==true)throw new Error('设备后端尚不支持自动代理。请先停止隧道并点击“安装 / 更新”，再保存。');if((p.sources.length||['gateway','lan','device'].includes(p.networkMode))&&!await confirm('保存流量接管设置？','启动后按所选范围修改本插件的路由、转发、NAT 和 DNS。IPv6 按所选策略处理，停止时撤销。'))return;const r=await KTSHost.save(p);state.saved=JSON.stringify(p);$('#validation').textContent='已保存。\n'+(r.warnings||[]).join('\n');await refresh();report('已保存；旧配置已自动创建快照。');},
    load:async()=>{if(!await confirm('读取设备配置？','配置包含私钥，并会显示在编辑器中。当前未保存的编辑将被替换。'))return;fill(await KTSHost.call('load'));report('设备配置已载入内存。');},
    start:async()=>{if(state.saved&&['gateway','lan','device'].includes(JSON.parse(state.saved).networkMode)&&state.status?.autoProxy!==true)throw new Error('请先更新设备后端，当前版本不能应用自动代理设置。');if(!await confirm('启动已保存的隧道？','将使用设备上已保存的代理范围、IPv6 和 DNS 设置。请确认已保存本次修改；启动可能短暂改变网络。'))return;await KTSHost.call('start');await refresh();report('隧道已启动，请查看握手状态。');},
    stop:async()=>{await KTSHost.call('stop');await refresh();report('本插件隧道已停止。');},
    'boot-on':async()=>{await KTSHost.call('boot-on');await refresh();report('已启用本插件开机自启。');},
    'boot-off':async()=>{await KTSHost.call('boot-off');await refresh();report('已关闭本插件开机自启。');},
    backup:async()=>{await KTSHost.call('backup');await listBackups();report('已创建配置快照。');},
    backups:listBackups,
    rollback:async()=>{if(!await confirm('回滚资源？','恢复上一份校验通过的资源，配置保持当前保存内容。'))return;await KTSHost.call('rollback');await refresh();report('资源已回滚。');},
    diagnose:async()=>{state.diagnostics=await KTSHost.call('diagnose');$('#diagnostics').textContent=JSON.stringify(state.diagnostics,null,2);report('只读诊断完成，不包含配置密钥。');},
    'download-diagnostics':async()=>{if(!state.diagnostics)throw new Error('请先生成诊断。');download('tunnel-studio-diagnostics.json',JSON.stringify(state.diagnostics,null,2));},
    export:async()=>{if(!await confirm('导出含密钥的配置？','下载的 JSON 含私钥，请保存在可信位置。'))return;download('tunnel-studio-profile.json',JSON.stringify(profile(),null,2));report('已导出当前编辑内容。');},
    clear:async()=>{if(!await confirm('清空本页编辑？','只清空浏览器内存中的编辑内容。'))return;$('#config').value='';$('#sources').value='';state.saved=null;controls();report('编辑器已清空。');}
  };
  $$('[data-action]').forEach(b=>b.addEventListener('click',()=>transaction(actions[b.dataset.action])));
  $$('[data-tab]').forEach(b=>b.addEventListener('click',()=>{$$('[data-tab]').forEach(x=>x.setAttribute('aria-selected',String(x===b)));$$('[data-panel]').forEach(x=>x.hidden=x.dataset.panel!=='profile'&&x.dataset.panel!==b.dataset.tab);}));
  $('.mode').onchange=e=>{shell.dataset.mode=e.target.value;try{localStorage.setItem('kts-display-mode',e.target.value)}catch{}};
  try{const m=localStorage.getItem('kts-display-mode');if(['auto','desktop','phone'].includes(m)){$('.mode').value=m;shell.dataset.mode=m}}catch{}
  for(const x of [$('#config'),$('#sources'),$('#network-mode'),$('#lan-interface'),$('#ipv6-mode'),...$$('input[name=protocol]')])x.addEventListener('input',()=>{state.touched=true;controls()});
  $('#import').onchange=()=>transaction(async()=>{const file=$('#import').files[0];if(!file)return;try{if(file.size>100000)throw new Error('配置文件超过 100 KB。');if(state.touched&&dirty()&&!await confirm('替换当前编辑？','尚未保存的编辑将丢失。'))return;const text=await file.text();if(file.name.endsWith('.json')){const p=JSON.parse(text);if(!['wireguard','amneziawg'].includes(p.protocol)||typeof p.text!=='string'||(p.networkMode!==undefined&&!['tunnel','sources','device','lan','gateway'].includes(p.networkMode))||(p.ipv6Mode!==undefined&&!['block','nat','routed'].includes(p.ipv6Mode))||(p.sources&&(!Array.isArray(p.sources)||p.sources.some(s=>typeof s!=='string'))))throw new Error('不是本插件的备份格式。');fill(p);state.saved=null;}else{$('#config').value=text;state.saved=null;}state.touched=true;report('已导入到编辑器，请检查协议并校验。');}finally{$('#import').value='';}});
  controls();
  const timer=setInterval(()=>{if(!host.isConnected){clearInterval(timer);return;}if(!document.hidden&&state.status?.installed&&!state.busy)transaction(async()=>{try{await refresh()}catch{report('状态刷新失败，设备状态暂时未知。',true);$('#connection').textContent='状态未知';state.status=null;controls();}})},10000);
})();
