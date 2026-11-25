"""Create a private, independent installation configuration without echoing secrets."""
from pathlib import Path
import argparse, json, os, secrets

parser = argparse.ArgumentParser()
parser.add_argument('--https', action='store_true', help='Enable secure cookies behind HTTPS')
args = parser.parse_args()
root = Path(__file__).resolve().parents[1]
target = root / 'config/config.json'
if target.exists():
    raise SystemExit('config/config.json already exists; it was not changed.')
config = json.loads((root / 'config/config.example.json').read_text(encoding='utf-8'))
state = root / config.get('state_directory', 'data')
database = root / config.get('sqlite', './data/mofufans.db').split('?', 1)[0]
if any((state / name).exists() for name in ('instance.json', 'settings.json')) or any(path.exists() and path.stat().st_size for path in (database, root / 'ask.db')):
    raise SystemExit('Existing instance data found. Keep its original keys; this initializer is only for a new installation.')
config['security'] = {'session_secret': secrets.token_urlsafe(48), 'bilibili_cookie_key': secrets.token_urlsafe(48), 'secure_cookie': args.https}
fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w', encoding='utf-8', newline='\n') as handle:
    json.dump(config, handle, ensure_ascii=False, indent=2)
    handle.write('\n')
print('Created config/config.json. Complete administrator setup at /admin/setup/. Back up this private configuration with your data.')
