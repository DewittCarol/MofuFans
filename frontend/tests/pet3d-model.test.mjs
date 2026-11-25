import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';
import { Euler, Vector3 } from 'three';

const source = readFileSync(new URL('../src/lib/pet3d-model.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText;
const { pet3dGaze, pet3dModels, resolvePet3dExpression } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`);

function facing(gaze) {
  // Normalized VRM forward (-Z), then the VRM0 scene rotation used by rotateVRM0.
  return new Vector3(0, 0, -1).applyEuler(new Euler(gaze.pitch, gaze.yaw, 0))
    .applyAxisAngle(new Vector3(0, 1, 0), Math.PI);
}

test('expression selection resolves VRM0 custom names and safely falls back if unsupported', () => {
  const names = ['neutral', 'happy', 'angry', 'sad', 'relaxed', 'Surprised'];
  assert.equal(resolvePet3dExpression(names, 'surprised'), 'Surprised');
  assert.equal(resolvePet3dExpression(names, 'happy'), 'happy');
  assert.equal(resolvePet3dExpression(names, 'missing'), 'neutral');
});

test('mouse below the model turns both its head and eye target downward, never upward', () => {
  const down = pet3dGaze(0, 1), up = pet3dGaze(0, -1);
  assert.ok(facing(down).y < 0, 'head must face toward negative world Y for the bottom of the screen');
  assert.ok(down.eyeY < 0);
  assert.ok(facing(up).y > 0); assert.ok(up.eyeY > 0);
  const neutral = facing(pet3dGaze(0, 0));
  assert.equal(neutral.y, 0);
});

test('horizontal following agrees with the rendered facing direction and input remains bounded', () => {
  assert.ok(facing(pet3dGaze(1, 0)).x > 0);
  assert.ok(facing(pet3dGaze(-1, 0)).x < 0);
  assert.deepEqual(pet3dGaze(200, -100), pet3dGaze(1, -1));
  assert.deepEqual(pet3dGaze(NaN, Infinity), pet3dGaze(0, 0));
});

test('distribution has no bundled character models', () => {
  assert.deepEqual(pet3dModels, []);
});
