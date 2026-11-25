"""Check only releasable source; never print matched credential values."""
from pathlib import Path
import json, re, subprocess, sys

root = Path(__file__).resolve().parents[1]
if (root / '.git').exists():
    result = subprocess.run(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=root, check=True, capture_output=True)
    files = sorted(set(filter(None, result.stdout.decode('utf-8').split('\0'))))
else:
    ignored = {'.git', 'node_modules', '.next', '.next-dev', 'out', 'dist', '__pycache__', 'data', '.test-runs'}
    files = [p.relative_to(root).as_posix() for p in root.rglob('*') if p.is_file() and not ignored.intersection(p.relative_to(root).parts)]

problems = []
private = re.compile(r'(^|/)(\.env(?:\..*)?|config\.json|data|uploads|\.ssh|\.connection|maintenance|recordings|website-assets)(/|$)|\.(?:db(?:-wal|-shm)?|sqlite3?|pem|key|pfx|p12|bundle|exe|log)$', re.I)
patterns = {
    'private-key material': re.compile(r'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----'),
    'cloud or GitHub token': re.compile(r'\b(?:LTAI[A-Za-z0-9]{16,}|gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})\b'),
    'credential-bearing URL': re.compile(r'https?://[^\s/\"\']+:[^\s/\"\']+@'),
}
for name in files:
    p = root / name
    if not p.is_file():
        continue
    if private.search(name) and not name.endswith('.env.example'):
        problems.append((name, 'private/runtime file'))
    if p.suffix in {'.png', '.jpg', '.jpeg', '.webp', '.gif', '.ico', '.zip'}:
        continue
    try:
        text = p.read_text(encoding='utf-8')
    except (UnicodeError, OSError):
        problems.append((name, 'unexpected binary file'))
        continue
    if name == 'scripts/check-release.py':
        continue
    for label, pattern in patterns.items():
        # Tests deliberately exercise invalid user:password URLs.
        if label == 'credential-bearing URL' and ('test' in name):
            continue
        if pattern.search(text):
            problems.append((name, label))
    if name.endswith('.example.json'):
        data = json.loads(text)
        def inspect(obj):
            if isinstance(obj, dict):
                for key, value in obj.items():
                    if key in {'password', 'password_hash', 'pass', 'session_secret', 'bilibili_cookie_key', 'access_key', 'secret_key', 'token', 'cookie'} and value:
                        problems.append((name, 'nonempty example credential'))
                    inspect(value)
            elif isinstance(obj, list):
                for value in obj:
                    inspect(value)
        inspect(data)
if problems:
    for name, reason in problems:
        print(f'{name}: {reason}')
    raise SystemExit(1)
print(f'Release content check passed ({len(files)} source files). No matched values were printed.')
