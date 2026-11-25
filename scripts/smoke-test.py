"""Exercise a fresh installation using temporary data and no external accounts."""
from pathlib import Path
import argparse, hashlib, http.cookiejar, json, os, secrets, shutil, socket
import sqlite3, subprocess, sys, tempfile, time, urllib.error, urllib.request

parser = argparse.ArgumentParser()
target = parser.add_mutually_exclusive_group(required=True)
target.add_argument('--binary', help='Path to the compiled MofuFans API')
target.add_argument('--native-package', help='Test a native deployment package and its nginx template')
parser.add_argument('--nginx', action='store_true', help='Also test built static exports through installed nginx')
args = parser.parse_args()
root = Path(__file__).resolve().parents[1]
package = Path(args.native_package).resolve() if args.native_package else None
binary = package / 'mofufans' if package else Path(args.binary).resolve()
source = package or root
if package:
    args.nginx = True

def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]

with tempfile.TemporaryDirectory(prefix='mofufans-smoke-') as directory:
    work = Path(directory)
    (work / 'config').mkdir()
    (work / 'scripts').mkdir()
    shutil.copy(source / 'config/config.example.json', work / 'config/config.example.json')
    shutil.copy(source / 'scripts/init-config.py', work / 'scripts/init-config.py')
    password = secrets.token_urlsafe(24)
    subprocess.run([sys.executable, 'scripts/init-config.py'], cwd=work, check=True, capture_output=True)
    config_path = work / 'config/config.json'
    config = json.loads(config_path.read_text())
    assert config['security']['session_secret'] != config['security']['bilibili_cookie_key']
    assert not config['bootstrap_admin']['password_hash']
    original = config_path.read_bytes()
    repeat = subprocess.run([sys.executable, 'scripts/init-config.py'], cwd=work, capture_output=True)
    assert repeat.returncode != 0 and config_path.read_bytes() == original
    config['server']['port'] = free_port()
    config['development'] = {'disable_background_workers': True}
    config_path.write_text(json.dumps(config))
    env = {key: value for key, value in os.environ.items() if not key.startswith('MOFUFANS_')}
    env.update(MOFUFANS_CONFIG=str(config_path), GIN_MODE='release')
    processes = []
    with (work / 'api.log').open('w') as log:
        try:
            api = subprocess.Popen([str(binary)], cwd=work, env=env, stdout=log, stderr=log)
            processes.append(api)
            base = f"http://127.0.0.1:{config['server']['port']}"
            for attempt in range(100):
                if api.poll() is not None:
                    raise RuntimeError('Fresh API exited before becoming ready')
                try:
                    with urllib.request.urlopen(base + '/api/theme', timeout=1) as response:
                        if response.status == 200:
                            break
                except (urllib.error.URLError, TimeoutError):
                    time.sleep(.1)
            else:
                raise RuntimeError('API did not become ready')
            if args.nginx:
                port = free_port()
                if package:
                    nginx = (package / 'deploy/nginx-native.conf').read_text()
                    nginx = nginx.replace('/opt/mofufans/web', str(package / 'web'))
                    nginx = nginx.replace('listen 127.0.0.1:8088;', f'listen 127.0.0.1:{port};')
                    nginx = 'events { worker_connections 128; }\nhttp {\ninclude /etc/nginx/mime.types;\n' + nginx + '\n}'
                else:
                    nginx = (root / 'nginx.conf').read_text()
                    nginx = nginx.replace('listen 80;', f'listen 127.0.0.1:{port};')
                nginx = nginx.replace('127.0.0.1:8080', f"127.0.0.1:{config['server']['port']}")
                if not package:
                    nginx = nginx.replace('root /work/frontend;', f'root {root}/frontend/out;')
                    # Match the image layout: /work/admin contains the admin export.
                    (work / 'admin').symlink_to(root / 'admin/out', target_is_directory=True)
                    nginx = nginx.replace('root /work;', f'root {work};')
                nginx = 'pid ' + str(work / 'nginx.pid') + ';\nerror_log stderr;\n' + nginx
                # Root-run tests keep synthetic secrets in 0700 temporary directories.
                # Only this loopback fixture uses a root worker; production templates do not.
                if hasattr(os, 'geteuid') and os.geteuid() == 0:
                    nginx = 'user root;\n' + nginx
                nginx = nginx.replace('http {', 'http {\n    access_log off;\n    client_body_temp_path ' + str(work / 'body') + ';\n    proxy_temp_path ' + str(work / 'proxy') + ';')
                conf = work / 'nginx.conf'
                conf.write_text(nginx)
                subprocess.run(['nginx', '-t', '-c', str(conf), '-p', str(work)], check=True, capture_output=True)
                processes.append(subprocess.Popen(['nginx', '-c', str(conf), '-p', str(work), '-g', 'daemon off;'], stdout=log, stderr=log))
                base = f'http://127.0.0.1:{port}'
                for attempt in range(50):
                    try:
                        urllib.request.urlopen(base + '/', timeout=1).close()
                        break
                    except urllib.error.URLError:
                        time.sleep(.1)
                for path in ['/', '/admin/', '/admin/setup/', '/gifts/', '/admin/dashboard/gifts/', '/admin/dashboard/voyage/shipping/']:
                    try:
                        with urllib.request.urlopen(base + path) as response:
                            assert response.status == 200 and 'text/html' in response.headers['Content-Type'], path
                    except urllib.error.HTTPError as error:
                        raise RuntimeError(f'Isolated proxy page {path} returned HTTP {error.code}') from error
            jar = http.cookiejar.CookieJar()
            client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
            def request(path, data=None):
                body = None if data is None else json.dumps(data).encode()
                req = urllib.request.Request(base + path, body, {'Content-Type': 'application/json'})
                with client.open(req, timeout=10) as response:
                    return json.load(response)
            assert request('/api/theme')['data']['theme']['brand']['site_name'] == 'MofuFans'
            token = (work / 'data/setup-token').read_text().strip()
            setup = {'username':'smoke-owner','password':password,'site_name':'MofuFans','streamer_name':'测试主播','settings':{'storage_type':'local','test_recipient_uid':'','secure_cookie':False,'oss':{}}}
            setup_req = urllib.request.Request(base+'/api/setup/complete', json.dumps(setup).encode(), {'Content-Type':'application/json','X-MofuFans-Request':'settings','X-Setup-Token':token})
            with client.open(setup_req) as response:
                assert json.load(response)['code'] == 200
            assert request('/api/info')['code'] != 200
            assert request('/api/login', {'username': 'admin', 'password': hashlib.md5(b'admin').hexdigest()})['code'] != 200
            assert request('/api/login', {'username': 'smoke-owner', 'password': hashlib.md5(password.encode()).hexdigest()})['code'] == 200
            assert request('/api/info')['data']['username'] == 'smoke-owner'
            catalog = request('/api/emoji-catalog')['data']
            assert any(pack['name'] == '轴伊Joi' for pack in catalog['collections'])
            assert sum(item['kind'] == 'input' and bool(item['url']) for item in catalog['emojis']) == 15
            assert sum(item['kind'] == 'reaction' and bool(item['url']) for item in catalog['emojis']) == 10
            if args.nginx:
                assets = json.loads((source / 'shared/joiask-emoji-assets.json').read_text())['assets']
                for asset in assets:
                    path = asset['path'].replace('frontend/public/', '/', 1).replace('admin/public/', '/admin/', 1)
                    with client.open(base + path, timeout=5) as response:
                        assert response.status == 200 and response.headers['Content-Type'].startswith('image/'), path
                        assert hashlib.sha256(response.read()).hexdigest() == asset['sha256'], path
            assert jar and all(cookie.has_nonstandard_attr('HttpOnly') for cookie in jar)
            for path in ['/api/does-not-exist', '/config/config.json', '/data/mofufans.db']:
                try:
                    client.open(base + path)
                    raise AssertionError('Removed/private route is exposed: ' + path)
                except urllib.error.HTTPError as error:
                    assert error.code == 404, path
            with sqlite3.connect(work / 'data/mofufans.db') as database:
                for table in ['questions', 'users', 'voyage_recipients', 'digital_gifts', 'gallery_works', 'songs']:
                    assert database.execute(f'SELECT COUNT(*) FROM {table}').fetchone()[0] == 0, table
            print('Fresh-install smoke passed: independent config, empty data, admin login, private routes' + (', static exports and nginx proxy.' if args.nginx else '.'))
        finally:
            for process in reversed(processes):
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
