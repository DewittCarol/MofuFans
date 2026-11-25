import type { VRM } from '@pixiv/three-vrm';

// These VRM0 models have long-hair hit spheres up to five times wider than
// the other strands. Calibrate only hair; keep the original model files intact.
export function configurePet3dPhysics(vrm: VRM, height: number) {
  for (const joint of vrm.springBoneManager?.joints ?? []) {
    if (!joint.bone.name.includes('Hair')) continue;
    joint.settings.dragForce = Math.max(joint.settings.dragForce, .7);
    joint.settings.stiffness = Math.max(joint.settings.stiffness, .8);
    joint.settings.hitRadius = Math.min(joint.settings.hitRadius, height*.012);
  }
}

export function pet3dSimulationSteps(elapsed: number) {
  if (!Number.isFinite(elapsed) || elapsed <= 0) return { count: 0, delta: 0 };
  const duration = Math.min(elapsed, .05);
  const count = Math.ceil(duration*60);
  return { count, delta: duration/count };
}
