// Deterministic UI harness. It is never included in the delivered plugin.
window.KANO_baseURL='/api';window.common_headers={'Authorization':'mock-only','Content-Type':'application/json'};
window.KTS_MOCK={installed:false,running:false,boot:false,fail:false,calls:[],uploads:new Map(),profile:null,backups:[],delay:0};
window.createToast=()=>{};
window.fetch=async(url,options)=>{if(!url.endsWith('/upload_img'))throw Error('unexpected network access');const file=options.body.get('file');const name='/uploads/mock-'+KTS_MOCK.uploads.size;KTS_MOCK.uploads.set('/data/data/com.minikano.f50_sms/files'+name,await file.text());return {ok:true,json:async()=>({url:name})}};
window.runShellWithRoot=async command=>{
 const s=KTS_MOCK;s.calls.push(command);if(s.delay)await new Promise(r=>setTimeout(r,s.delay));
 const token=/KTS_\d+_\d+/.exec(command)?.[0];if(!token)throw Error('missing sentinel');
 const exit=(data,code=0)=>({success:true,content:(typeof data==='string'?data:JSON.stringify(data))+'\n'+token+':'+code+'\n'});
 if(s.fail)return {success:false,content:'transport failed'};
 if(command.includes('uname -m'))return exit(command.includes('id -u')?'0\narmv8l\narm64-v8a':'uid=0(root) gid=0(root)\narmv8l\nAndroid 64-bit ABI: arm64-v8a\n/dev/tun');
 if(command.includes('release.json')&&command.includes('cat '))return s.installed?exit({current:'a'.repeat(64),previous:'b'.repeat(64)}):exit('not installed',1);
 const reply=data=>exit({ok:true,data});
 if(command.includes(' install')){s.installed=true;return reply({release:'a'.repeat(64)})}
 const clean=command.replace(/'\\''/g,"'");const action=/studio' '(.*?)'/.exec(clean)?.[1];
 if(action==='status')return reply({installed:s.installed,autoProxy:!s.legacyBackend,networkCheck:!s.legacyBackend,running:s.running,configured:!!s.profile,protocol:s.profile?.protocol,boot:s.boot,release:{current:'a'.repeat(64),previous:'b'.repeat(64)},tunnel:{networkMode:s.profile?.networkMode||'tunnel',ipv6Mode:s.profile?.ipv6Mode||'',dnsRedirect:false,peers:s.running?[{rx_bytes:'10240',tx_bytes:'2048',last_handshake_time_sec:String(Math.floor(Date.now()/1000)-3)}]:[]}});
 if(action==='load')return reply(s.profile);
 if(action==='save-file'||action==='validate-file'){const path=/studio' '[^']+' '([^']+)'/.exec(clean)?.[1];const p=JSON.parse(s.uploads.get(path));if(!p.text.includes('PrivateKey'))return exit({ok:false,error:'PrivateKey 无效'},1);if(action==='save-file')s.profile=p;return reply({warnings:['仅隧道模式']})}
 if(action==='start'){s.running=true;return reply(null)}if(action==='stop'){s.running=false;return reply(null)}
 if(action==='boot-on'){s.boot=true;return reply(null)}if(action==='boot-off'){s.boot=false;return reply(null)}
 if(action==='backup'){s.backups.push('20261012-test.json');return reply('20261012-test.json')}
 if(action==='backups')return reply(s.backups);
 if(action==='diagnose')return reply({uid:0,tun:true,platform:'UI mock; not a device test'});
 if(action==='network-check')return reply({targets:[],samples:[{sequence:1}],platform:'UI mock; not a device test'});
 if(action==='rollback'||action==='restore')return reply(null);
 if(command.includes('rm -f'))return exit('');
 return exit({ok:false,error:'unhandled mock action: '+String(action)},1);
};
