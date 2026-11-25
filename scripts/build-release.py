"""Build a portable Linux deployment directory without copying runtime data."""
import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile


def build(root, output, version='development'):
    if platform.system() != 'Linux':
        raise SystemExit('Build the native package on Linux, or in WSL / a Linux VM.')
    if output.exists():
        raise SystemExit(f'{output} already exists. Choose a new --output directory; existing installations are never overwritten.')
    for tool in ('node', 'npm', 'go'):
        if not shutil.which(tool):
            raise SystemExit(f'Missing {tool}; see doc/DEPLOYMENT.md for build requirements.')
    if subprocess.check_output(['go', 'env', 'GOOS'], text=True).strip() != 'linux':
        raise SystemExit('GOOS must be linux; build natively for the target architecture with CGO enabled.')
    for app in ('frontend', 'admin'):
        subprocess.run(['npm', 'ci', '--prefix', app], cwd=root, check=True)
        subprocess.run(['npm', 'run', 'build', '--prefix', app], cwd=root, check=True)
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.mofufans-build-', dir=output.parent) as directory:
        package = Path(directory) / 'mofufans'
        package.mkdir()
        subprocess.run(['go', 'build', '-trimpath', '-o', str(package / 'mofufans'), './cmd'],
                       cwd=root, env={**os.environ, 'CGO_ENABLED': '1'}, check=True)
        for app in ('frontend', 'admin'):
            shutil.copytree(root / app / 'out', package / 'web' / app)
        for name in ('config/config.example.json', 'scripts/init-config.py',
                     'deploy/nginx-native.conf', 'deploy/mofufans.service',
                     'deploy/Caddyfile', 'LICENSE', 'NOTICE.md', 'shared/joiask-emoji-assets.json',
                     'SECURITY.md', 'doc/DEPLOYMENT.md', 'doc/CONFIGURATION.md',
                     'doc/DEVELOPMENT.md', 'doc/AI.md', 'doc/FEATURES.md', 'doc/UPGRADING.md',
                     'doc/SECURITY.md', 'doc/SECURITY-OPERATIONS.md', 'doc/QUICKSTART.md', 'CHANGELOG.md'):
            target = package / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(root / name, target)
        shutil.copytree(root / 'scripts/whisper-helper', package / 'scripts/whisper-helper')
        shutil.copy2(root / 'deploy/AGENTS-native.md', package / 'AGENTS.md')
        readme = (root / 'deploy/README-native.md').read_text(encoding='utf-8')
        (package / 'README.md').write_text(readme.replace('](../', ']('), encoding='utf-8')
        metadata = {'version': version,
                    'revision': os.environ.get('MOFUFANS_BUILD_COMMIT') or os.environ.get('GITHUB_SHA') or 'source-archive',
                    'os': 'linux',
                    'arch': subprocess.check_output(['go', 'env', 'GOARCH'], text=True).strip(),
                    'libc': list(platform.libc_ver())}
        (package / 'BUILD.json').write_text(json.dumps(metadata, indent=2) + '\n', encoding='utf-8')
        (package / 'data').mkdir(mode=0o700)
        package.rename(output)
    print(f'Built {output}\nNext: follow doc/DEPLOYMENT.md to initialize and run this directory.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', default='dist/mofufans', help='New output directory (relative to the repository)')
    parser.add_argument('--version', default='development', help='Label embedded in BUILD.json; does not change configuration or data')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    build(root, (root / args.output).resolve(), args.version)
