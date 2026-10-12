"""Create the reviewable source/distribution bundle; never include private inputs."""
import pathlib,zipfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
target=ROOT.parent/'tunnel-studio-0.1.3.zip'
exclude={'node_modules','vendor-source','.git','work','test-results','__pycache__'}
with zipfile.ZipFile(target,'w',zipfile.ZIP_DEFLATED,compresslevel=9) as z:
    for p in sorted(ROOT.rglob('*')):
        rel=p.relative_to(ROOT)
        if not p.is_file() or any(part in exclude for part in rel.parts) or p.suffix=='.exe':continue
        z.write(p,'tunnel-studio/'+rel.as_posix())
print(target)
