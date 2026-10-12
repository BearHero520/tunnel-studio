"""Build a self-contained JS distribution from fresh, pinned sources."""
import base64,gzip,hashlib,json,os,pathlib,re,subprocess,sys
from prepare import ROOT,prepare,SOURCES
def run(*args,env=None): subprocess.run(args,cwd=ROOT,env=env,check=True)
def build():
    prepare()
    run('go','mod','verify')
    for mod in ['golang.org/x/crypto','golang.org/x/net','golang.org/x/sys','golang.zx2c4.com/wintun']:
        data=json.loads(subprocess.check_output(['go','list','-m','-json',mod],cwd=ROOT,text=True));p=pathlib.Path(data['Dir'])/'LICENSE'
        if p.exists():(ROOT/'licenses'/(mod.replace('/','_')+'.txt')).write_bytes(p.read_bytes())
    env=os.environ.copy();env.update(GOOS='android',GOARCH='arm64',CGO_ENABLED='0',GOTOOLCHAIN='go1.25.0')
    run('go','build','-trimpath','-buildvcs=false','-ldflags=-s -w -buildid=','-o','resources/studio-arm64','./cmd/studio',env=env)
    binary=(ROOT/'resources/studio-arm64').read_bytes()
    if binary[:4]!=b'\x7fELF' or binary[18:20]!=b'\xb7\x00':raise RuntimeError('Not an ARM64 ELF')
    digest=hashlib.sha256(binary).hexdigest()
    resource={'sha256':digest,'compression':'gzip','base64':base64.b64encode(gzip.compress(binary,compresslevel=9,mtime=0)).decode()}
    # Preserve third-party permission notices even when only the JS is redistributed.
    notices=[('Tunnel Studio', (ROOT/'LICENSE').read_text(encoding='utf-8'))]
    notices += [(p.name,p.read_text(encoding='utf-8')) for p in sorted((ROOT/'licenses').glob('*.txt'))]
    header='// Tunnel Studio 0.1.1 | Offline Android ARM64 | Built from pinned official sources\n/*\n'+ '\n\n'.join(name+'\n'+text for name,text in notices).replace('*/','* /')+'\n*/\n'
    script=header+'(()=>{\nconst KTS_RESOURCE='+json.dumps(resource,separators=(',',':'))+';\nconst KTS_STYLE='+json.dumps((ROOT/'src/style.css').read_text(encoding='utf-8'),ensure_ascii=False)+';\n'
    script+='\n'.join((ROOT/'src'/p).read_text(encoding='utf-8') for p in ('host.js','plugin.js'))+'\n})();\n'
    # KANO parses uploads as HTML and executes only extracted script elements.
    # JS comments around the tags also preserve direct JavaScript loading.
    if re.search(r'</script|<!--|<script',script,re.I):
        raise RuntimeError('Unsafe HTML script delimiter in bundled source')
    script='//<script>\n'+script+'//</script>\n'
    (ROOT/'dist/tunnel-studio.js').write_text(script,encoding='utf-8',newline='\n')
    metadata={'name':'Tunnel Studio','version':'0.1.1','target':'android/arm64','protocolModes':['wireguard','amneziawg'],'backend':dict(zip(('name','url','commit'),SOURCES[0])),'wireguardTestPeer':dict(zip(('name','url','commit'),SOURCES[1])),'binarySHA256':digest,'pluginSHA256':hashlib.sha256(script.encode()).hexdigest(),'goVersion':'go1.25.0','patches':['Android missing-node fallback to /dev/tun','Refresh S4 after initial blocking TUN read'], 'notes':'WireGuard mode uses the standard-compatible AWG userspace backend; kernel WG interfaces are not adopted.'}
    (ROOT/'resources/manifest.json').write_text(json.dumps(metadata,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (ROOT/'dist/SHA256SUMS').write_text(metadata['pluginSHA256']+'  tunnel-studio.js\n',encoding='ascii')
    info=subprocess.check_output(['go','version','-m',str(ROOT/'resources/studio-arm64')],text=True)
    info=info.replace(str(ROOT/'resources/studio-arm64'),'resources/studio-arm64',1)
    (ROOT/'resources/build-info.txt').write_text(info,encoding='utf-8')
    run('node','--check','dist/tunnel-studio.js')
    print(json.dumps({'binaryBytes':len(binary),'pluginBytes':len(script.encode()),'sha256':digest}))
if __name__=='__main__':build()
