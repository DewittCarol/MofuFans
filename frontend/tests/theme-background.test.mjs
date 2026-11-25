import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const css = read('../src/app/globals.css');
const linen = css.match(/\.fabric-linen\s*\{([^}]+)\}/)?.[1];
const defaults = JSON.parse(read('../../internal/theme/default.json'));
const source = read('../../shared/theme.ts').replace(
  /import defaults from ['"]\.\.\/internal\/theme\/default\.json['"];?/,
  `const defaults = ${JSON.stringify(defaults)};`,
);
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 },
}).outputText;
const { themeVariables } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`);

test('linen leaves the theme background visible and uses themed weave colors', () => {
  assert.ok(linen, 'linen style must exist');
  assert.match(linen, /background-color:\s*var\(--background\)/);
  assert.equal((linen.match(/repeating-linear-gradient\(/g) || []).length, 2);
  assert.match(linen, /transparent/);
  assert.match(linen, /var\(--fabric-grain\)/);
  assert.match(linen, /background-size:\s*4px 4px,\s*4px 4px/);
  assert.doesNotMatch(linen, /url\(|#[\da-f]{3,8}\b|rgba?\(/i, 'no fixed-color image or literal may cover the configured color');
  assert.doesNotMatch(css, /fabric-texture\.svg/, 'the old opaque pink texture must not be loaded');
});

test('changing background and weave colors updates independent frontend tokens', () => {
  for (const [background, grain] of [['#FFF8F2', '#F4E2D5'], ['#F2F6FA', '#E5EDF4'], ['#101820', '#314250']]) {
    const theme = structuredClone(defaults);
    theme.colors.background = background;
    theme.colors['fabric-grain'] = grain;
    theme.admin_colors.background = '#ABCDEF';
    for (const enabled of [false, true]) {
      theme.appearance.texture = enabled;
      const variables = themeVariables(theme, false);
      assert.equal(variables['--background'], background);
      assert.equal(variables['--fabric-grain'], grain);
    }
  }
});

test('texture preference is present before hydration and remains switchable', () => {
  assert.match(read('../src/app/layout.tsx'), /data-theme-texture=\{String\(defaultTheme\.appearance\.texture\)\}/);
  assert.match(read('../src/lib/site-theme.tsx'), /root\.dataset\.themeTexture=String\(value\.theme\.appearance\.texture\)/);
  assert.match(css, /\[data-theme-texture="false"\] body,\s*\[data-theme-texture="false"\] \.fabric-linen,[^{]+\{\s*background-image:\s*none;/);
});
