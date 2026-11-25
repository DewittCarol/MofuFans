from pathlib import Path
import zipfile

root = Path(__file__).resolve().parents[1]
source = root / 'scripts' / 'whisper-helper'
target = root / 'admin' / 'public' / 'tools'
target.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(target / 'fans-whisper-helper.zip', 'w', zipfile.ZIP_DEFLATED) as archive:
    for name in ['manifest.json', 'background.js', 'fill-editor.js', 'content.js', 'README.md']:
        archive.write(source / name, f'fans-whisper-helper/{name}')
parts = ['userscript-header.txt', 'fill-editor.js', 'userscript-bridge.js']
(target / 'fans-whisper-helper.user.js').write_text('\n\n'.join((source / name).read_text(encoding='utf-8') for name in parts), encoding='utf-8')
print('Built extension ZIP and userscript in admin/public/tools')
