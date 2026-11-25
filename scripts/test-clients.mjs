import { readdirSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
for (const [cwd, directory] of [['admin', 'tests'], ['frontend', 'tests'], ['.', 'scripts']]) {
  const base = resolve(root, cwd);
  const files = readdirSync(resolve(base, directory)).filter(name => /\.test\.(cjs|mjs)$/.test(name)).map(name => resolve(base, directory, name));
  const result = spawnSync(process.execPath, ['--test', ...files], { cwd: base, stdio: 'inherit' });
  if (result.status !== 0) process.exit(result.status ?? 1);
}
