/* A small KANO adapter; no credentials or device configuration are persisted by the UI. */
const KTSHost = (() => {
  const ROOT = '/data/adb/kano-tunnel-studio';
  const quote = s => "'" + String(s).replace(/'/g, "'\\''") + "'";
  const sanitize = text => String(text).replace(/(?:[A-Za-z0-9+/]{43}=)/g, '[密钥已隐藏]').replace(/\b[0-9a-f]{64}\b/gi, '[摘要]');
  function unpack(raw, token) {
    const match = new RegExp('(?:^|\\n)' + token + ':(\\d+)\\s*$').exec(raw);
    if (!match) throw new Error('设备命令没有完整结束；请刷新确认状态，不要重复点击。');
    const body = raw.slice(0, match.index).trim();
    if (Number(match[1]) !== 0) {
      let message = '设备操作失败';
      try { message = JSON.parse(body).error || message; } catch { message = sanitize(body).slice(-400) || message; }
      throw new Error(message);
    }
    return body;
  }
  async function shell(script, timeout = 30000) {
    if (typeof runShellWithRoot !== 'function') throw new Error('请在 KANO 管理页运行插件。');
    const token = 'KTS_' + crypto.getRandomValues(new Uint32Array(2)).join('_');
    const command = `sh -c ${quote(script)}; kts_rc=$?; printf '\\n${token}:%s\\n' "$kts_rc"`;
    const response = await runShellWithRoot(command, timeout);
    if (!response || typeof response.content !== 'string') throw new Error('KANO 没有返回命令结果。');
    return unpack(response.content, token);
  }
  function uploadedPath(url) {
    if (typeof url !== 'string' || !/^\/?[A-Za-z0-9_-]+(?:\/[A-Za-z0-9_.-]+)*$/.test(url) || url.includes('..') || url.split('/').includes('.')) throw new Error('上传路径无效。');
    return '/data/data/com.minikano.f50_sms/files/' + url.replace(/^\//, '');
  }
  async function upload(blob, filename) {
    if (typeof KANO_baseURL === 'undefined' || typeof common_headers === 'undefined') throw new Error('KANO 缺少文件上传接口。');
    const body = new FormData(); body.append('file', new File([blob], filename, {type: 'application/octet-stream'}));
    const headers = new Headers(common_headers); headers.delete('Content-Type');
    const abort = new AbortController(); const timer = setTimeout(() => abort.abort(), 90000);
    try {
      const r = await fetch(`${KANO_baseURL}/upload_img`, { method: 'POST', headers, body, signal: abort.signal });
      if (!r.ok) throw new Error(`上传失败：HTTP ${r.status}`);
      return uploadedPath((await r.json()).url);
    } finally { clearTimeout(timer); }
  }
  async function binary() {
    const s = await shell(`test ! -L ${quote(ROOT)} && test -f ${quote(ROOT + '/owner.json')} && cat ${quote(ROOT + '/release.json')}`);
    const r = JSON.parse(s);
    if (!/^[a-f0-9]{64}$/.test(r.current)) throw new Error('设备资源清单无效。');
    const path = `${ROOT}/releases/${r.current}/studio`;
    // Verify every invoked executable before execution, including version rollback.
    return {path, hash:r.current};
  }
  function verified(path, hash) {
    return `test ! -L ${quote(path)} && test "$(sha256sum ${quote(path)} | cut -d ' ' -f 1)" = ${quote(hash)}`;
  }
  async function call(action, arg = '') {
    const {path,hash} = await binary();
    const raw = await shell(`${verified(path,hash)} && ${quote(path)} ${quote(action)}${arg ? ' ' + quote(arg) : ''}`, 45000);
    const reply = JSON.parse(raw); if (!reply.ok) throw new Error(reply.error || '设备操作失败'); return reply.data;
  }
  async function install(resource) {
    const identity = await shell('id -u; uname -m');
    if (!/^0\s*\n(?:aarch64|arm64)\s*$/.test(identity)) throw new Error('安装需要 KANO 的 root 通道和 ARM64 设备。');
    const bytes = Uint8Array.from(atob(resource.base64), c => c.charCodeAt(0));
    if (typeof DecompressionStream !== 'function') throw new Error('当前浏览器缺少离线解压能力，请使用新版 Chrome / Edge。');
    const blob = await new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream('gzip'))).blob();
    const path = await upload(blob, 'kts-' + resource.sha256.slice(0,16) + '.bin');
    const stage = '/data/local/tmp/kts-stage-' + crypto.getRandomValues(new Uint32Array(2)).join('-');
    const command = `umask 077\nmkdir ${quote(stage)} || exit 1\ntrap ${quote('rm -f ' + quote(stage+'/studio') + '; rmdir ' + quote(stage))} EXIT\n` +
      `test ! -L ${quote(path)} && cp ${quote(path)} ${quote(stage+'/studio')} && ${verified(stage+'/studio',resource.sha256)} && chmod 700 ${quote(stage+'/studio')} && ${quote(stage+'/studio')} install`;
    try { return JSON.parse(await shell(command, 90000)); }
    finally { await shell(`test ! -L ${quote(path)} && rm -f ${quote(path)}`).catch(() => {}); }
  }
  async function save(profile, validateOnly = false) {
    const path = await upload(new Blob([JSON.stringify(profile)], {type:'application/json'}), 'kts-profile-' + Date.now() + '.json');
    try { return await call(validateOnly ? 'validate-file' : 'save-file',path); }
    finally { await shell(`test ! -L ${quote(path)} && rm -f ${quote(path)}`).catch(() => {}); }
  }
  return {call,install,save,shell,quote,unpack,uploadedPath,sanitize};
})();
if (typeof module !== 'undefined') module.exports = KTSHost;
