// Run in the Console of your already signed-in KANO page. Never share tokens.
// Only the exact published v0.1.0 plugin block is removed. No root commands.
(async()=>{
  const expected='b81f5e7d0ab0a47255337f4b31bfe6cf22da1c49df916b37f89358e39aaeb6e7';
  if(typeof common_headers==='undefined'||typeof CryptoJS==='undefined')
    throw new Error('请先在 KANO 页面登录，等待宿主脚本加载后再运行。');
  const read=async()=>{
    const r=await fetch('/api/get_custom_head',{headers:common_headers});
    if(!r.ok)throw new Error('读取失败：HTTP '+r.status);
    const data=await r.json();
    if(typeof data.text!=='string')throw new Error('插件列表格式异常，未作修改。');
    return data.text;
  };
  const before=await read();let removed=0;
  const after=before.replace(/<!-- \[KANO_PLUGIN_START\] ([^\r\n]*?) -->\r?\n([\s\S]*?)\r?\n<!-- \[KANO_PLUGIN_END\] \1 -->/g,(block,name,body)=>{
    if(!body.startsWith('// Tunnel Studio 0.1.0 |'))return block;
    const normalized=body.replace(/\r\n/g,'\n').trimEnd()+'\n';
    if(CryptoJS.SHA256(normalized).toString()!==expected)return block;
    removed++;return '';
  });
  if(!removed)throw new Error('未找到哈希匹配的故障版本，未作修改。');
  if(await read()!==before)throw new Error('插件列表已变化，未作修改，请重试。');
  const response=await fetch('/api/set_custom_head',{
    method:'POST',headers:{...common_headers,'Content-Type':'application/json'},body:JSON.stringify({text:after})
  });
  if(!response.ok)throw new Error('保存失败：HTTP '+response.status+'；请确认登录状态。');
  const result=await response.json();
  if(result.result!=='success'||result.error)throw new Error('服务端未确认保存成功。');
  if((await read()).trim()!==after.trim())throw new Error('保存后校验未通过，请勿继续导入。');
  console.log('已移除 '+removed+' 个故障版本块，其他插件已保留。现在可刷新 KANO 页面。');
})().catch(e=>console.error(e.message));
