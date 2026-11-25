"""Rehearse upgrade using two binaries and synthetic data in a temporary directory."""
import argparse, hashlib, http.cookiejar, json, os
from pathlib import Path
import secrets, socket, sqlite3, subprocess, tempfile, time, urllib.error, urllib.request

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--old-binary',required=True)
parser.add_argument('--binary',required=True)
parser.add_argument('--expect-legacy-session-revoked',action='store_true',help='Use only when the old binary creates the former unbound admin session.')
args=parser.parse_args()
old=Path(args.old_binary).resolve();new=Path(args.binary).resolve()
with socket.socket() as sock:
    sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
base=f'http://127.0.0.1:{port}'
with tempfile.TemporaryDirectory(prefix='mofufans-upgrade-') as temp:
    work=Path(temp);(work/'data').mkdir()
    password=secrets.token_urlsafe(24)
    config={'db_type':'sqlite','sqlite':'./data/mofufans.db','state_directory':'./data','storage_type':'local','server':{'host':'127.0.0.1','port':port},'security':{'session_secret':secrets.token_urlsafe(48),'bilibili_cookie_key':secrets.token_urlsafe(48),'secure_cookie':False},'development':{'disable_background_workers':True}}
    configfile=work/'config.json';configfile.write_text(json.dumps(config))
    os.chmod(configfile,0o600)
    env={k:v for k,v in os.environ.items() if not k.startswith('MOFUFANS_')}
    env.update(MOFUFANS_CONFIG=str(configfile),GIN_MODE='release')
    client=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    def request(path,body=None,method=None,token=''):
        req=urllib.request.Request(base+path,None if body is None else json.dumps(body).encode(),{'Content-Type':'application/json','X-MofuFans-Request':'settings','X-Setup-Token':token},method=method)
        with client.open(req,timeout=10) as response:return json.load(response)
    process=None
    log=(work/'api.log').open('w')
    def start(binary):
        global process
        process=subprocess.Popen([str(binary)],cwd=work,env=env,stdout=log,stderr=log)
        for _ in range(120):
            if process.poll() is not None:raise RuntimeError('Upgrade fixture API exited before startup')
            try:
                if request('/api/theme')['code']==200:return
            except (urllib.error.URLError,TimeoutError):pass
            time.sleep(.1)
        raise RuntimeError('Upgrade fixture API did not start')
    def stop():
        if process is not None:
            process.terminate()
            try:process.wait(timeout=5)
            except subprocess.TimeoutExpired:process.kill();process.wait()
    try:
        start(old)
        token=(work/'data/setup-token').read_text().strip()
        assert request('/api/setup/complete',{'username':'upgrade-owner','password':password,'site_name':'Upgrade fixture','streamer_name':'Test streamer','settings':{'storage_type':'local','test_recipient_uid':'123456','secure_cookie':False,'oss':{}}},token=token)['code']==200
        login={'username':'upgrade-owner','password':hashlib.md5(password.encode()).hexdigest()}
        assert request('/api/login',login)['code']==200
        original_theme=request('/api/theme')['data']['theme']
        original_theme['brand']['site_name']='Upgrade fixture'
        original_theme['colors']['primary']='#126789'
        original_theme['modules']['ask']['label']='测试入口'
        stop()
        dbpath=work/'data/mofufans.db'
        with sqlite3.connect(dbpath) as db:
            db.execute('UPDATE site_themes SET config=?,revision=7 WHERE id=1',(json.dumps(original_theme),))
            def seed(table,values):
                columns={row[1] for row in db.execute(f'PRAGMA table_info({table})')}
                assert values.keys()<=columns,table
                names=','.join(values)
                db.execute(f'INSERT INTO {table} ({names}) VALUES ({",".join("?" for _ in values)})',list(values.values()))
            seed('questions',{'id':101,'content':'offline existing question','tag_id':1,'is_publish':True})
            seed('voyage_rosters',{'id':201,'title':'offline roster','category':'monthly','month':'2026-01','version':4})
            seed('voyage_roster_members',{'id':301,'roster_id':201,'uid':'12345','name':'offline member','tier':'captain','version':3})
            seed('digital_gifts',{'id':401,'title':'offline gift','mode':'open','version':2})
            seed('voyage_issues',{'id':601,'kind':'batch','title':'offline batch','month':'2026-01','version':2})
            seed('voyage_batches',{'id':601,'title':'offline batch','sources':json.dumps([{'roster_id':201,'tiers':['captain']}]),'version':2})
            seed('voyage_recipients',{'id':501,'issue_id':601,'uid':'12345','tier':'captain','name':'offline member','code_hash':'offline-hash','code_encrypted':'offline-cipher','version':3})
            seed('voyage_deliveries',{'recipient_id':501,'address_encrypted':'offline-address-snapshot','version':8})
            keep={table:db.execute(f'SELECT * FROM {table}').fetchall() for table in ['questions','voyage_rosters','voyage_roster_members','digital_gifts','voyage_issues','voyage_batches','voyage_recipients','voyage_deliveries']}
            db.commit()
            backup=work/'before.sqlite'
            with sqlite3.connect(backup) as copy:db.backup(copy)
        start(new)
        assert request('/api/setup/status')['data']['required'] is False
        previous_session=request('/api/info')
        if args.expect_legacy_session_revoked:
            assert previous_session['code']!=200,'old unbound admin session remains valid'
        else:
            assert previous_session['code']==200 and previous_session['data']['username']=='upgrade-owner','valid bound admin session was lost'
        assert request('/api/login',login)['code']==200
        theme=request('/api/theme')['data']
        assert theme['revision']==7 and theme['theme']['brand']['site_name']=='Upgrade fixture'
        assert theme['theme']['colors']['primary']=='#126789'
        assert theme['theme']['modules']['ask']['label']=='测试入口'
        assert theme['theme']['modules']['gifts']['label']=='电子礼物'
        assert request('/api/voyage/rosters')['code']==200
        assert request('/api/admin/gifts')['code']==200
        assert configfile.read_text()==json.dumps(config),'deployment configuration replaced'
        with sqlite3.connect(dbpath) as db,sqlite3.connect(backup) as before:
            assert db.execute('PRAGMA quick_check').fetchone()[0]=='ok'
            for table,rows in keep.items():
                columns=[r[1] for r in before.execute(f'PRAGMA table_info({table})')]
                current=db.execute(f'SELECT {",".join(columns)} FROM {table}').fetchall()
                assert current==rows,f'original records changed: {table}'
            assert db.execute('SELECT password FROM admins WHERE username=?',('upgrade-owner',)).fetchone()[0].startswith('$2')
            for table in ['signature_challenges','voyage_address_book_entries','voyage_address_consents','voyage_long_term_addresses']:
                assert db.execute(f'SELECT COUNT(*) FROM {table}').fetchone()[0]==0,table
        stop();start(new)
        assert request('/api/info')['data']['username']=='upgrade-owner'
        assert request('/api/theme')['data']['revision']==7
        session_result='legacy unbound session revoked' if args.expect_legacy_session_revoked else 'valid bound session retained'
        print('Upgrade smoke passed: existing theme and records preserved, original keys retained, '+session_result+', bcrypt password retained or upgraded, no address authorization added, repeated startup valid.')
    finally:
        stop();log.close()
