(async () => {
  'use strict';
  const id='kano-tunnel-studio';
  if(document.getElementById(id)) return;
  let anchor;
  for(let n=0;n<60;n++){anchor=document.querySelector('.functions-container');if(anchor)break;await new Promise(r=>setTimeout(r,100));}
  if(!anchor)return;
  const host=document.createElement('section');host.id=id;anchor.append(host);
  const ui=host.attachShadow({mode:'open'});
  const style=document.createElement('style');style.textContent=KTS_STYLE;ui.append(style);
  const shell=document.createElement('div');shell.className='studio';shell.dataset.mode='auto';
  shell.innerHTML=`
  <header class="head"><div class="brand"><span class="mark" aria-hidden="true">↗</span><div><div class="eyebrow">KANO / NETWORK</div><h1>Tunnel Studio</h1></div></div><select class="mode" aria-label="显示模式"><option value="auto">自动布局</option><option value="desktop">桌面模式</option><option value="phone">手机模式</option></select></header>
  <div class="body"><nav aria-label="隧道管理"><button data-tab="overview" aria-selected="true">连接概览</button><button data-tab="profile" aria-selected="false">协议与配置</button><button data-tab="recovery" aria-selected="false">版本与恢复</button><button data-tab="diagnostics" aria-selected="false">只读诊断</button><div class="nav-foot">一个控制台<br>两种协议<br><br>v0.1.1 · ARM64</div></nav>
  <main>
  <section data-panel="overview"><div class="topline"><div><h2>让连接保持清晰。</h2><p class="muted">WireGuard 与 AmneziaWG，独立管理。</p></div><span class="badge" id="protocol-badge">未选择协议</span></div>
    <div class="status"><div><span class="muted"><span class="dot" id="dot"></span>本插件隧道</span><div class="big" id="connection">尚未读取状态</div><div class="hint" id="status-detail">点击刷新，检查设备资源和运行情况。</div></div><div class="row"><button data-action="refresh">刷新</button><button class="primary" data-action="start">启动</button><button data-action="stop">停止</button></div></div>
    <div class="metrics"><div class="metric"><span class="muted">已接收</span><strong id="rx">—</strong></div><div class="metric"><span class="muted">已发送</span><strong id="tx">—</strong></div><div class="metric"><span class="muted">最近握手</span><strong id="handshake">—</strong></div></div>
    <div class="card"><div class="topline"><div><h3>开机自动连接</h3><p class="muted">通过独立的 Magisk service.d 入口启动已保存配置。</p></div><span class="badge" id="boot-state">未读取</span></div><div class="row"><button data-action="boot-on">开启自启</button><button data-action="boot-off">关闭自启</button></div></div>
    <p class="note" style="margin-top:16px">默认仅创建独立隧道。需要代理特定来源网段时，在配置页明确启用来源路由；转发权限及防火墙由现有系统管理。</p>
  </section>
  <section data-panel="profile" hidden><div class="topline"><div><h2>协议与配置</h2><p class="muted">先选协议，再导入或编辑。切换协议不会转换密钥与参数。</p></div><span class="badge" id="dirty">尚未载入</span></div>
    <div class="stack"><div class="protocols"><label><input type="radio" name="protocol" value="wireguard">WireGuard<br><span class="hint">标准协议 · 严格拒绝 AWG 扩展</span></label><label><input type="radio" name="protocol" value="amneziawg" checked>AmneziaWG<br><span class="hint">混淆与 3.x 安全扩展</span></label></div>
    <div class="card"><div class="editor-title"><label for="config">隧道配置</label><button data-action="load">读取设备配置</button></div><textarea id="config" spellcheck="false" autocomplete="off" aria-label="隧道配置" placeholder="[Interface]&#10;PrivateKey = &lt;你的私钥&gt;&#10;Address = 10.0.0.2/32&#10;&#10;[Peer]&#10;PublicKey = &lt;服务端公钥&gt;&#10;AllowedIPs = 0.0.0.0/0, ::/0&#10;Endpoint = &lt;服务端地址&gt;:51820"></textarea><div class="hint">配置仅保留在本页内存与设备私有目录；不会写入浏览器存储。DNS 字段仅供参考，本版不改全局 DNS。</div><label for="import">从本地导入 .conf 或本插件备份 .json</label><input id="import" type="file" accept=".conf,.json,text/plain,application/json"></div>
    <div class="card"><h3>流量范围</h3><label for="sources">来源网段（可选，每行一个 CIDR）</label><textarea id="sources" style="min-height:78px" rows="2" spellcheck="false" placeholder="留空：仅隧道，不添加策略路由"></textarea><p class="hint">填写后将把这些来源、且命中 AllowedIPs 的流量送入隧道。使用独立路由表，支持 IPv4 / IPv6，不依赖 NAT66。请先在设备上配置所需转发与服务端回程路由。</p></div>
    <div class="row"><button data-action="validate">校验配置</button><button data-action="save" class="primary">保存到设备</button><button data-action="export">导出当前编辑</button><button data-action="clear">清空编辑</button></div><pre class="note" id="validation">固定官方后端负责最终校验。安装资源后可校验并保存。</pre></div>
  </section>
  <section data-panel="recovery" hidden><div class="topline"><div><h2>每次变更，都可回看。</h2><p class="muted">资源版本与配置备份分别管理。</p></div></div><div class="stack">
    <div class="card"><h3>离线资源</h3><p class="muted">JS 已内置 Android ARM64 后端，安装不访问下载服务器。升级与回滚需先停止本插件连接。</p><div class="version" id="release">设备版本：未读取</div><div class="row" style="margin-top:16px"><button class="primary" data-action="install">安装 / 更新内置资源</button><button data-action="rollback">回滚上一资源版本</button></div></div>
    <div class="card"><h3>配置快照</h3><p class="muted">保存和恢复前自动保留旧配置；备份含私钥，保存在设备 0700 私有目录。</p><div class="row"><button data-action="backup">创建快照</button><button data-action="backups">读取快照列表</button></div><div id="backups" style="margin-top:12px"><p class="hint">尚未读取快照。</p></div></div>
  </div></section>
  <section data-panel="diagnostics" hidden><h2>观察设备，不改网络。</h2><p class="muted">检查执行权限、TUN、转发状态和本插件的运行信息。诊断不读取私钥。</p><div class="row"><button data-action="preflight">检查 KANO 执行通道</button><button class="primary" data-action="diagnose">生成只读诊断</button><button data-action="download-diagnostics">导出诊断</button></div><div class="card" style="margin-top:20px"><pre id="diagnostics">等待检查。</pre></div><p class="note">普通 ADB 没有 root 不代表 KANO 没有权限。以这里返回的执行身份为准。</p></section>
  </main></div><div class="message" role="status" aria-live="polite">准备就绪。首次使用请先检查 KANO 执行通道，再安装内置资源。</div>
  <dialog><h2 id="dialog-title"></h2><p id="dialog-text"></p><div class="row"><button id="cancel">取消</button><button class="primary" id="accept">继续</button></div></dialog>`;
  ui.append(shell);
  const $=s=>ui.querySelector(s),$$=s=>[...ui.querySelectorAll(s)];
  const state={busy:false,status:null,saved:null,touched:false,diagnostics:null};
  const profile=()=>({protocol:$('input[name=protocol]:checked').value,text:$('#config').value,sources:$('#sources').value.split(/\r?\n/).map(s=>s.trim()).filter(Boolean)});
  const dirty=()=>state.saved!==JSON.stringify(profile());
  const report=(text,error=false)=>{$('.message').textContent=KTSHost.sanitize(text);$('.message').classList.toggle('error',error);};
  function controls(){
    const s=state.status,run=Boolean(s?.running);
    $$('button[data-action]').forEach(b=>{const a=b.dataset.action;b.disabled=state.busy||(['save','restore','install','rollback','start'].includes(a)&&run)||(['save','validate','start','boot-on','boot-off','backup','backups','load','rollback','diagnose'].includes(a)&&!s?.installed)||(a==='start'&&(!s?.configured||(state.touched&&dirty())))||(a==='stop'&&!run);});
    $('#dirty').textContent=state.touched&&dirty()?'有未保存修改':state.saved===null?'尚未载入':'已与设备同步';
  }
  function fill(p){$('input[value='+p.protocol+']').checked=true;$('#config').value=p.text;$('#sources').value=(p.sources||[]).join('\n');state.saved=JSON.stringify(profile());state.touched=false;controls();}
  function bytes(n){if(!Number.isFinite(n))return '—';if(n<1024)return n+' B';if(n<1048576)return (n/1024).toFixed(1)+' KiB';return (n/1048576).toFixed(1)+' MiB';}
  function paint(s){state.status=s;const running=Boolean(s.running);$('#connection').textContent=running?'隧道已启动':s.installed?'已停止':'尚未安装';$('#dot').classList.toggle('on',running);$('#status-detail').textContent=running?'已创建本插件隧道；是否连通请结合握手与计数判断。':s.configured?'配置已保存，可以启动。':'安装资源并保存配置后即可启动。';$('#protocol-badge').textContent=s.protocol==='wireguard'?'WireGuard':s.protocol==='amneziawg'?'AmneziaWG':'未选择协议';$('#boot-state').textContent=s.boot?'已开启':'未开启';$('#release').textContent=s.release?'当前资源：'+s.release.current+'\n上一资源：'+(s.release.previous||'无'):'尚未安装';const peers=s.tunnel?.peers||[];$('#rx').textContent=running?bytes(peers.reduce((a,p)=>a+Number(p.rx_bytes||0),0)):'—';$('#tx').textContent=running?bytes(peers.reduce((a,p)=>a+Number(p.tx_bytes||0),0)):'—';const h=Math.max(0,...peers.map(p=>Number(p.last_handshake_time_sec||0)));$('#handshake').textContent=h?Math.max(0,Math.floor(Date.now()/1000-h))+' 秒前':running?'尚未握手':'—';controls();}
  async function refresh(){try{paint(await KTSHost.call('status'))}catch(e){state.status=null;$('#connection').textContent='状态未知';$('#dot').classList.remove('on');controls();throw e}}
  async function confirm(title,text){const d=$('dialog');$('#dialog-title').textContent=title;$('#dialog-text').textContent=text;return new Promise(resolve=>{const done=value=>{d.close();$('#cancel').onclick=null;$('#accept').onclick=null;d.oncancel=null;resolve(value);};$('#cancel').onclick=()=>done(false);$('#accept').onclick=()=>done(true);d.oncancel=e=>{e.preventDefault();done(false)};d.showModal();$('#cancel').focus();});}
  async function transaction(fn){if(state.busy)return;state.busy=true;controls();try{await fn();}catch(e){report(e.message||'操作未完成',true);}finally{state.busy=false;controls();}}
  function download(name,contents,type='application/json'){const url=URL.createObjectURL(new Blob([contents],{type}));const a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),30000);}
  async function listBackups(){const names=await KTSHost.call('backups');const box=$('#backups');box.replaceChildren();if(!names?.length){box.textContent='暂无快照。';return;}for(const name of names.slice().reverse()){const row=document.createElement('div');row.className='backup-item';const label=document.createElement('span');label.textContent=name;const b=document.createElement('button');b.textContent='恢复';b.disabled=Boolean(state.status?.running);b.onclick=()=>transaction(async()=>{if(!await confirm('恢复这份快照？','将替换设备配置，当前设备配置会另存快照。未保存的编辑也将被替换。'))return;await KTSHost.call('restore',name);fill(await KTSHost.call('load'));await refresh();await listBackups();report('快照已恢复，尚未启动连接。');});row.append(label,b);box.append(row);}}
  const actions={
    refresh:async()=>{await refresh();report('设备状态已刷新。');},
    preflight:async()=>{const text=await KTSHost.shell('id; uname -m; ls -l /dev/net/tun /dev/tun 2>/dev/null; true');state.diagnostics={execution:text};$('#diagnostics').textContent=text;report('执行通道检查完成。');},
    install:async()=>{if(!await confirm('安装内置资源？','将写入 Tunnel Studio 独立目录。升级时保留配置与上一版本；不会自动启动隧道。'))return;report('正在上传并校验内置资源，请保持页面打开。');const r=await KTSHost.install(KTS_RESOURCE);if(!r.ok)throw new Error(r.error||'安装未完成');await refresh();report('资源已安装。可导入配置后校验。');},
    validate:async()=>{const r=await KTSHost.save(profile(),true);$('#validation').textContent='校验通过。\n'+(r.warnings||[]).join('\n');report('配置校验通过，尚未保存。');},
    save:async()=>{const p=profile();if(p.sources.length&&!await confirm('保存来源路由设置？','启动后会影响所列来源网段中命中 AllowedIPs 的流量。请确认转发和回程路由已准备好。'))return;const r=await KTSHost.save(p);state.saved=JSON.stringify(p);$('#validation').textContent='已保存。\n'+(r.warnings||[]).join('\n');await refresh();report('已保存；旧配置已自动创建快照。');},
    load:async()=>{if(!await confirm('读取设备配置？','配置包含私钥，并会显示在编辑器中。当前未保存的编辑将被替换。'))return;fill(await KTSHost.call('load'));report('设备配置已载入内存。');},
    start:async()=>{if(!await confirm('启动已保存的隧道？','将创建 kts0 接口，并应用配置中明确填写的来源路由。'))return;await KTSHost.call('start');await refresh();report('隧道已启动，请查看握手状态。');},
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
  $$('[data-tab]').forEach(b=>b.addEventListener('click',()=>{$$('[data-tab]').forEach(x=>x.setAttribute('aria-selected',String(x===b)));$$('[data-panel]').forEach(x=>x.hidden=x.dataset.panel!==b.dataset.tab);}));
  $('.mode').onchange=e=>{shell.dataset.mode=e.target.value;try{localStorage.setItem('kts-display-mode',e.target.value)}catch{}};
  try{const m=localStorage.getItem('kts-display-mode');if(['auto','desktop','phone'].includes(m)){$('.mode').value=m;shell.dataset.mode=m}}catch{}
  for(const x of [$('#config'),$('#sources'),...$$('input[name=protocol]')])x.addEventListener('input',()=>{state.touched=true;controls()});
  $('#import').onchange=()=>transaction(async()=>{const file=$('#import').files[0];if(!file)return;try{if(file.size>100000)throw new Error('配置文件超过 100 KB。');if(state.touched&&dirty()&&!await confirm('替换当前编辑？','尚未保存的编辑将丢失。'))return;const text=await file.text();if(file.name.endsWith('.json')){const p=JSON.parse(text);if(!['wireguard','amneziawg'].includes(p.protocol)||typeof p.text!=='string'||(p.sources&&(!Array.isArray(p.sources)||p.sources.some(s=>typeof s!=='string'))))throw new Error('不是本插件的备份格式。');fill(p);state.saved=null;}else{$('#config').value=text;state.saved=null;}state.touched=true;report('已导入到编辑器，请检查协议并校验。');}finally{$('#import').value='';}});
  controls();
  const timer=setInterval(()=>{if(!host.isConnected){clearInterval(timer);return;}if(!document.hidden&&state.status?.installed&&!state.busy)transaction(async()=>{try{await refresh()}catch{report('状态刷新失败，设备状态暂时未知。',true);$('#connection').textContent='状态未知';state.status=null;controls();}})},10000);
})();
