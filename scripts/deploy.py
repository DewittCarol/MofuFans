"""Start MofuFans with Docker Compose, then complete setup in the browser."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--https', action='store_true', help='Override secure cookies for an HTTPS deployment')
    parser.add_argument('--check', action='store_true', help='Check prerequisites without writing files or starting containers')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    docker = shutil.which('docker')
    if not docker:
        raise SystemExit('Docker is missing. Install Docker Engine / Desktop and the Compose plugin first.')
    for command in ([docker, 'compose', 'version'], [docker, 'info', '--format', '{{.ServerVersion}}']):
        result = subprocess.run(command, capture_output=True, text=True)
        if result.returncode:
            raise SystemExit('Docker / Compose is not ready. Start Docker and check the current user has access to its daemon.')
    if args.check:
        print('Docker and Compose are ready. No files were changed and no containers were started.')
        return
    env = os.environ.copy()
    if args.https:
        env['MOFUFANS_SECURITY_SECURE_COOKIE'] = 'true'
        print('HTTPS cookies are enabled for this container. Use an HTTPS address to sign in.')
    subprocess.run([docker, 'compose', 'up', '-d', '--build'], cwd=root, env=env, check=True)
    print('Started. Open http://localhost:8088/ and /admin/ locally, or use your configured HTTPS domain.')
    print('Logs: docker compose logs --tail=100; stop: docker compose down')
    print('First launch: copy the one-time installation code from the logs, then open /admin/ to create your account.')


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        raise SystemExit(f'Deployment stopped (exit {error.returncode}); inspect the command output above.') from None
