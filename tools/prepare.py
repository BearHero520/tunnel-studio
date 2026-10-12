"""Acquire fixed upstream sources; never read or derive from another plugin."""
import pathlib, subprocess
ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCES = [
    ('amneziawg-go', 'https://github.com/amnezia-vpn/amneziawg-go.git', 'b5928efb6ca19f0153958460c3d141f04abc5c2e'),
    ('wireguard-go', 'https://github.com/WireGuard/wireguard-go.git', '2631ce99a06f27120d581611cf125d68bc6aa565'),
]
def run(*args, cwd=None):
    return subprocess.check_output(args, cwd=cwd, text=True).strip()
def prepare():
    for name, url, commit in SOURCES:
        dest=ROOT/'vendor-source'/name
        if not dest.exists():
            dest.parent.mkdir(parents=True, exist_ok=True)
            run('git','init',str(dest))
            run('git','remote','add','origin',url,cwd=dest)
            run('git','fetch','--depth','1','origin',commit,cwd=dest)
            run('git','checkout','--detach','FETCH_HEAD',cwd=dest)
        if run('git','rev-parse','HEAD',cwd=dest)!=commit: raise RuntimeError('Source pin mismatch')
        allowed={'device/send.go','tun/tun_linux.go'} if name=='amneziawg-go' else set()
        changed=set(filter(None,run('git','diff','--name-only','HEAD',cwd=dest).splitlines()))
        untracked=run('git','ls-files','--others','--exclude-standard',cwd=dest)
        if changed-allowed or untracked: raise RuntimeError('Unexpected local changes in fixed source: '+name)
    awg=ROOT/'vendor-source/amneziawg-go'
    changes={
        'tun/tun_linux.go':(
            'nfd, err := unix.Open(cloneDevicePath, unix.O_RDWR|unix.O_CLOEXEC, 0)',
            'nfd, err := unix.Open(cloneDevicePath, unix.O_RDWR|unix.O_CLOEXEC, 0)\n\t// Tunnel Studio: Android fallback only for a missing standard node.\n\tif os.IsNotExist(err) {\n\t\tnfd, err = unix.Open("/dev/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)\n\t}'),
        'device/send.go':(
            'count, readErr = device.tun.device.Read(bufs, sizes, offset)',
            '''count, readErr = device.tun.device.Read(bufs, sizes, offset)
		// Tunnel Studio: the first blocking Read may precede the initial IpcSet.
		// Refresh S4 and relocate payload before handing buffers to encryption.
		newPadding := device.paddings.transport.Load()
		newOffset := MessageTransportHeaderSize + int(newPadding)
		if newOffset != offset {
			for i := 0; i < count; i++ {
				if sizes[i] < 0 || newOffset+sizes[i] > len(bufs[i]) {
					sizes[i] = 0
					continue
				}
				copy(bufs[i][newOffset:], bufs[i][offset:offset+sizes[i]])
			}
			padding, offset = newPadding, newOffset
		}'''),
    }
    for name,(old,new) in changes.items():
        path=awg/name
        raw=run('git','show','HEAD:'+name,cwd=awg)+'\n'
        if raw.count(old)!=1: raise RuntimeError('Patch context mismatch: '+name)
        path.write_text(raw.replace(old,new),encoding='utf-8',newline='\n')
    (ROOT/'resources/upstream.patch').write_text(run('git','diff','--no-ext-diff',cwd=awg)+'\n',encoding='utf-8',newline='\n')
    (ROOT/'licenses/AmneziaWG-MIT.txt').write_bytes((awg/'LICENSE').read_bytes())
    (ROOT/'licenses/WireGuard-test-only-MIT.txt').write_bytes((ROOT/'vendor-source/wireguard-go/LICENSE').read_bytes())
if __name__=='__main__': prepare()
