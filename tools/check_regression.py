"""Reproduce the initial-S4 defect against unmodified upstream, then restore patch."""
import pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
path=ROOT/'vendor-source/amneziawg-go/device/send.go'
patched=path.read_bytes()
original=subprocess.check_output(['git','show','HEAD:device/send.go'],cwd=path.parent.parent)
try:
    path.write_bytes(original)
    p=subprocess.run(['go','test','./tests','-run','TestEncryptedInteroperability/amneziawg-obfuscation','-count=1','-timeout=12s'],cwd=ROOT,capture_output=True,text=True)
    (ROOT/'docs/regression-unpatched.txt').write_text(p.stdout+p.stderr,encoding='utf-8')
    if p.returncode==0 or 'first packet lost' not in p.stdout:raise RuntimeError('Expected first-packet failure was not reproduced')
finally:
    path.write_bytes(patched)
subprocess.run(['go','test','./tests','-run','TestEncryptedInteroperability','-count=1','-timeout=30s'],cwd=ROOT,check=True)
print('Unpatched failure reproduced; patched three-mode interoperability passed.')
