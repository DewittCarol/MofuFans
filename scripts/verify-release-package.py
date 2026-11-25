"""Check the actual distributable, including CPU ABI, private-data exclusions and local document links."""
import argparse,json,re,struct,urllib.parse
from pathlib import Path
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('directory',type=Path)
parser.add_argument('--arch',choices=['amd64','arm64'],required=True)
parser.add_argument('--version',required=True)
args=parser.parse_args();root=args.directory.resolve();errors=[]
required=['mofufans','BUILD.json','README.md','AGENTS.md','SECURITY.md','LICENSE','NOTICE.md','web/frontend/index.html','web/admin/index.html','config/config.example.json','deploy/nginx-native.conf','deploy/mofufans.service','doc/SECURITY.md','doc/SECURITY-OPERATIONS.md','doc/UPGRADING.md','doc/FEATURES.md']
for name in required:
    if not (root/name).is_file():errors.append('Missing '+name)
metadata=json.loads((root/'BUILD.json').read_text())
if metadata['arch']!=args.arch or metadata['version']!=args.version or metadata['os']!='linux':errors.append('Build identity mismatch')
data=(root/'mofufans').read_bytes()
if data[:6]!=b'\x7fELF\x02\x01' or struct.unpack_from('<H',data,18)[0]!={'amd64':62,'arm64':183}[args.arch]:errors.append('ELF architecture mismatch')
if any((root/'data').iterdir()):errors.append('Runtime data is not empty')
for path in root.rglob('*'):
    if not path.is_file():continue
    name=path.relative_to(root).as_posix()
    if re.search(r'\.(?:db|sqlite3?|pem|key|pfx|log)$',name,re.I) or name in ['config/config.json','data/instance.json','data/settings.json','data/setup-token']:errors.append('Private/runtime file: '+name)
    if path.suffix!='.md':continue
    for match in re.finditer(r'\[[^\]]+\]\(([^\s\)]+)\)',path.read_text(encoding='utf-8')):
        link=urllib.parse.urlsplit(urllib.parse.unquote(match.group(1)).strip('<>'))
        if link.scheme or link.netloc or not link.path:continue
        target=(path.parent/link.path).resolve()
        if not target.is_relative_to(root) or not target.exists():errors.append('Broken package link: '+name+' -> '+match.group(1))
if errors:
    raise SystemExit('\n'.join(errors))
print('Verified package: '+args.arch+', '+args.version+', empty runtime data, complete documents and working local links.')
