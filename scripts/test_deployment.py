"""Safety checks for packaging and the deployment entry point; no real Docker calls."""
import contextlib
import importlib.util
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


def load(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


builder = load('build-release')
deployer = load('deploy')


class DeploymentTests(unittest.TestCase):
    def test_package_excludes_existing_config_and_data(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            files = ('config/config.example.json', 'scripts/init-config.py', 'deploy/nginx-native.conf',
                     'deploy/mofufans.service', 'deploy/Caddyfile', 'deploy/README-native.md',
                     'LICENSE', 'NOTICE.md', 'shared/joiask-emoji-assets.json', 'doc/DEPLOYMENT.md', 'doc/CONFIGURATION.md',
                     'SECURITY.md', 'deploy/AGENTS-native.md', 'scripts/whisper-helper/README.md',
                     'doc/DEVELOPMENT.md', 'doc/AI.md', 'doc/FEATURES.md', 'doc/UPGRADING.md',
                     'doc/SECURITY.md', 'doc/SECURITY-OPERATIONS.md', 'doc/QUICKSTART.md', 'CHANGELOG.md',
                     'frontend/out/index.html', 'admin/out/index.html')
            for name in files:
                file = root / name
                file.parent.mkdir(parents=True, exist_ok=True)
                file.write_text('public fixture', encoding='utf-8')
            (root / 'config/config.json').write_text('PRIVATE CONFIG MUST STAY HERE')
            (root / 'data').mkdir()
            (root / 'data/private.db').write_text('PRIVATE DATABASE MUST STAY HERE')
            output = root / 'dist/mofufans'
            def run(command, **kwargs):
                if command[0] == 'go':
                    Path(command[command.index('-o') + 1]).write_bytes(b'binary fixture')
                return subprocess.CompletedProcess(command, 0)
            with patch.object(builder.platform, 'system', return_value='Linux'), \
                 patch.object(builder.shutil, 'which', return_value='/tools/test'), \
                 patch.object(builder.subprocess, 'check_output', return_value='linux\n'), \
                 patch.object(builder.subprocess, 'run', side_effect=run), \
                 contextlib.redirect_stdout(io.StringIO()):
                builder.build(root, output)
            self.assertTrue((output / 'mofufans').is_file())
            self.assertTrue((output / 'web/admin/index.html').is_file())
            self.assertTrue((output / 'doc/SECURITY-OPERATIONS.md').is_file())
            self.assertTrue((output / 'doc/UPGRADING.md').is_file())
            self.assertTrue((output / 'AGENTS.md').is_file())
            self.assertFalse((output / 'config/config.json').exists())
            self.assertEqual(list((output / 'data').iterdir()), [])
            self.assertEqual((root / 'config/config.json').read_text(), 'PRIVATE CONFIG MUST STAY HERE')
            with patch.object(builder.platform, 'system', return_value='Linux'), \
                 patch.object(builder.subprocess, 'run') as commands:
                with self.assertRaises(SystemExit):
                    builder.build(root, output)
                commands.assert_not_called()

    def run_deployer(self, root, options, command_result=0):
        commands = []
        def run(command, **kwargs):
            commands.append(command)
            return subprocess.CompletedProcess(command, command_result, '', '')
        with patch.object(deployer, '__file__', str(root / 'scripts/deploy.py')), \
             patch.object(deployer.shutil, 'which', return_value='/tools/docker'), \
             patch.object(deployer.subprocess, 'run', side_effect=run), \
             patch.object(deployer.sys, 'argv', ['deploy.py'] + options), \
             contextlib.redirect_stdout(io.StringIO()):
            deployer.main()
        return commands

    def test_check_only_does_not_initialize_or_start(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            commands = self.run_deployer(root, ['--check'])
            self.assertEqual(list(root.iterdir()), [])
            self.assertEqual(len(commands), 2)
            self.assertFalse(any('up' in command for command in commands))

    def test_existing_config_is_never_overwritten(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'config').mkdir()
            config = root / 'config/config.json'
            config.write_text('existing-secret-fixture')
            commands = self.run_deployer(root, ['--https'])
            self.assertEqual(config.read_text(), 'existing-secret-fixture')
            self.assertFalse(any('init-config.py' in ' '.join(command) for command in commands))
            self.assertEqual(commands[-1][1:], ['compose', 'up', '-d', '--build'])

    def test_first_launch_needs_no_config_or_interactive_credentials(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            commands = self.run_deployer(root, [])
            self.assertEqual(list(root.iterdir()), [])
            self.assertEqual(commands[-1][1:], ['compose', 'up', '-d', '--build'])
            self.assertFalse(any('init-config.py' in ' '.join(command) for command in commands))

    def test_unavailable_daemon_stops_before_writing(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(SystemExit):
                self.run_deployer(root, [], command_result=1)
            self.assertEqual(list(root.iterdir()), [])


if __name__ == '__main__':
    unittest.main()
