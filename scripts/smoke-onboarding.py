"""Test browser onboarding and restart persistence with temporary data, no external services."""
import argparse
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True)
    args = parser.parse_args()
    binary = Path(args.binary).resolve()
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    base = f'http://127.0.0.1:{port}'
    env = {k: v for k, v in os.environ.items() if not k.startswith('MOFUFANS_')}
    env.update(MOFUFANS_SERVER_PORT=str(port), MOFUFANS_DEVELOPMENT_DISABLE_BACKGROUND_WORKERS='true', GIN_MODE='release')
    jar = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

    def request(path, body=None, token='', method=None):
        headers = {'Content-Type': 'application/json', 'X-MofuFans-Request': 'settings'}
        if token:
            headers['X-Setup-Token'] = token
        req = urllib.request.Request(base + path, None if body is None else json.dumps(body).encode(), headers, method=method)
        with client.open(req, timeout=15) as response:
            return json.load(response)

    with tempfile.TemporaryDirectory(prefix='mofufans-onboarding-') as directory:
        work = Path(directory)
        with (work / 'api.log').open('w') as log:
            process = None

            def start():
                nonlocal process
                process = subprocess.Popen([str(binary)], cwd=work, env=env, stdout=log, stderr=log)
                for _ in range(100):
                    if process.poll() is not None:
                        raise RuntimeError('API exited during zero-config startup')
                    try:
                        return request('/api/setup/status')['data']['required']
                    except (urllib.error.URLError, TimeoutError):
                        time.sleep(.1)
                raise RuntimeError('API did not become ready')

            def stop():
                if process is not None:
                    process.terminate()
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()

            try:
                assert start() is True
                assert not (work / 'config/config.json').exists()
                keys = (work / 'data/instance.json').read_bytes()
                parsed = json.loads(keys)
                assert len(set(parsed.values())) == 2
                try:
                    request('/api/gifts')
                    raise AssertionError('Business API open before setup')
                except urllib.error.HTTPError as error:
                    assert error.code == 503
                assert request('/api/setup/verify', {}, 'wrong')['code'] == 403
                token = (work / 'data/setup-token').read_text().strip()
                settings = request('/api/setup/verify', {}, token)['data']
                assert settings['storage_type'] == 'local' and settings['test_recipient_uid'] == ''
                assert 'secret_key' not in settings['oss'] and 'access_key' not in settings['oss']
                values = {key: settings[key] for key in ('storage_type', 'test_recipient_uid', 'secure_cookie')}
                values['oss'] = {key: settings['oss'][key] for key in ('endpoint', 'region', 'bucket', 'prefix')}
                password = secrets.token_urlsafe(20)
                body = {'username': 'smoke-owner', 'password': password, 'site_name': 'Smoke fans', 'streamer_name': 'Smoke streamer', 'settings': values}
                assert request('/api/setup/complete', body, token)['code'] == 200
                assert request('/api/setup/complete', body, token)['code'] == 409
                assert not (work / 'data/setup-token').exists()
                assert request('/api/theme')['data']['theme']['brand']['site_name'] == 'Smoke fans'
                login = {'username': 'smoke-owner', 'password': hashlib.md5(password.encode()).hexdigest()}
                assert request('/api/login', login)['code'] == 200
                assert request('/api/info')['data']['is_super_admin'] is True
                catalog = request('/api/emoji-catalog')['data']
                common = next(item for item in catalog['collections'] if item['name'] == '常用表情')
                common_assets = [item for item in catalog['emojis'] if item['collection_id'] == common['id']]
                delete_path = f"/api/emoji-collections/{common['id']}"
                unauthenticated = urllib.request.Request(base + delete_path, method='DELETE')
                with urllib.request.urlopen(unauthenticated) as response:
                    assert json.load(response)['code'] == 408
                assert request(delete_path, method='DELETE')['code'] == 200
                ai_settings = request('/api/ai-settings')['data']
                assert ai_settings.pop('api_key_configured') is False
                assert ai_settings['enabled'] is False
                ai_settings.update(api_key='smoke-test-placeholder-key', model='custom-model', clear_key=False)
                saved_ai = request('/api/ai-settings', ai_settings, method='PUT')
                assert saved_ai['code'] == 200 and saved_ai['data']['api_key_configured'] is True
                assert 'smoke-test-placeholder-key' not in json.dumps(saved_ai)
                footer = {'footer_icp_number': 'Smoke ICP', 'footer_police_number': 'Smoke 12345678901234', 'footer_police_url': '', 'footer_police_icon': ''}
                assert request('/api/footer-settings', footer, method='PUT')['code'] == 200
                assert request('/api/appearance', {'cursor_theme': 'system'}, method='PUT')['code'] == 200
                assert request('/api/settings', {'allow_repeated_reactions': True}, method='PUT')['code'] == 200
                public = request('/api/config')['data']
                assert public['footer_icp_number'] == 'Smoke ICP'
                assert public['footer_police_url'].endswith('code=12345678901234')
                assert 'ai_key_ciphertext' not in public and 'api_key' not in public
                values['test_recipient_uid'] = '123456789'
                assert request('/api/installation-settings', values, method='PUT')['code'] == 200
                assert request('/api/installation-settings')['data']['test_recipient_uid'] == '123456789'
                stop()
                assert start() is False
                assert (work / 'data/instance.json').read_bytes() == keys
                assert request('/api/info')['data']['username'] == 'smoke-owner'
                restarted_catalog = request('/api/emoji-catalog')['data']
                assert all(item['id'] != common['id'] for item in restarted_catalog['collections'])
                for original in common_assets:
                    restored = next(item for item in restarted_catalog['emojis'] if item['id'] == original['id'])
                    assert restored['is_deleted'] is True and restored['value'] == original['value'] and restored['url'] == original['url']
                assert request('/api/installation-settings')['data']['test_recipient_uid'] == '123456789'
                assert request('/api/gifts')['code'] == 200
                assert request('/api/config')['data']['footer_icp_number'] == 'Smoke ICP'
                restored_ai = request('/api/ai-settings')['data']
                assert restored_ai['api_key_configured'] is True and restored_ai['model'] == 'custom-model' and restored_ai['enabled'] is False
                print('Browser onboarding smoke passed: zero-config boot, ownership gate, admin login, collection deletion, AI and footer settings, restart persistence.')
            finally:
                stop()


if __name__ == '__main__':
    main()
