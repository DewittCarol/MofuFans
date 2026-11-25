import * as THREE from 'three';
import { VRMSpringBoneJoint, type VRM } from '@pixiv/three-vrm';

// Each joint bends a little, with a damped response delayed down the strand.
// Displacement accumulates toward the tip without using the misplaced colliders.
class SoftHairJoint extends VRMSpringBoneJoint {
  private rest: THREE.Quaternion;
  private lag = new THREE.Quaternion();
  private headWorld = new THREE.Quaternion();
  private relative = new THREE.Quaternion();
  private offset = new THREE.Quaternion();
  private euler = new THREE.Euler(0, 0, 0, 'YXZ');
  private angle = new THREE.Vector3();
  private velocity = new THREE.Vector3();
  private target = new THREE.Vector3();
  private time = 0;
  private started = false;
  constructor(source: VRMSpringBoneJoint, bone: THREE.Object3D, child: THREE.Object3D | null,
    private head: THREE.Object3D, private depth: number, private rebuilt = true) {
    super(bone, child, { ...source.settings }, []);
    this.rest = bone.quaternion.clone();
    head.getWorldQuaternion(this.lag);
  }
  override reset() {
    super.reset(); this.head.getWorldQuaternion(this.lag);
    this.angle.set(0, 0, 0); this.velocity.set(0, 0, 0); this.time = 0; this.started = true;
  }
  override update(delta: number) {
    if (!Number.isFinite(delta) || delta <= 0) return;
    const count = Math.ceil(Math.min(delta, .05) * 120), dt = Math.min(delta, .05) / count;
    this.head.getWorldQuaternion(this.headWorld);
    // The loader rotates VRM0 after repairing the rig; that initial orientation
    // change is not a head movement and must not kick the hair on first display.
    if (!this.started) { this.lag.copy(this.headWorld); this.started = true; }
    for (let step = 0; step < count; step++) {
      this.time += dt;
      this.lag.slerp(this.headWorld, 1 - Math.exp(-dt * (7 - Math.min(this.depth, 9) * .38)));
      this.relative.copy(this.headWorld).invert().multiply(this.lag);
      this.euler.setFromQuaternion(this.relative, 'YXZ');
      const gain = this.rebuilt ? (this.depth === 0 ? .52 : .13 + this.depth * .008) : .2;
      const limit = this.rebuilt && this.depth === 0 ? .14 : .065;
      const breeze = Math.sin(this.time * 1.65 - this.depth * .42) * (this.depth === 0 ? .008 : .0035);
      this.target.set(this.euler.x * gain + breeze * .35, this.euler.y * gain * .45,
        (this.euler.z + this.euler.y * .6) * gain + breeze).clampScalar(-limit, limit);
      const stiffness = 48 - Math.min(this.depth, 9) * 2, damping = 2 * Math.sqrt(stiffness) * .64;
      for (const axis of ['x', 'y', 'z'] as const) {
        this.velocity[axis] += ((this.target[axis] - this.angle[axis]) * stiffness - this.velocity[axis] * damping) * dt;
        this.angle[axis] = THREE.MathUtils.clamp(this.angle[axis] + this.velocity[axis] * dt, -limit, limit);
      }
    }
    this.euler.set(this.angle.x, this.angle.y, this.angle.z, 'YXZ');
    this.bone.quaternion.copy(this.rest).multiply(this.offset.setFromEuler(this.euler));
    this.bone.updateMatrix(); this.bone.updateWorldMatrix(false, false);
  }
}

type Attribute = THREE.BufferAttribute | THREE.InterleavedBufferAttribute;
type HairSurface = {
  meshes: THREE.SkinnedMesh[]; position: Attribute; indices: Attribute; weights: Attribute;
  source: number[]; mirror: number[];
};

// Follow connected ribbons through their formerly Head-only upper sections.
// Welding seams prevents UV/material splits from leaving stationary patches.
function findHairSurface(meshes: THREE.SkinnedMesh[], chain: Set<THREE.Bone>): HairSurface | null {
  const { geometry, skeleton } = meshes[0];
  const position = geometry.getAttribute('position'), indices = geometry.getAttribute('skinIndex'), weights = geometry.getAttribute('skinWeight');
  if (!position || !indices || !weights) return null;
  const count = position.count, parents = Int32Array.from({ length: count }, (_, i) => i);
  const find = (i: number): number => {
    while (parents[i] !== i) { parents[i] = parents[parents[i]]; i = parents[i]; }
    return i;
  };
  const join = (a: number, b: number) => { parents[find(a)] = find(b); };
  const cells = new Map<string, number[]>(), unit = 1e-5;
  const key = (x: number, y: number, z: number) => `${x},${y},${z}`;
  for (let i = 0; i < count; i++) {
    const cell = key(Math.round(position.getX(i) / unit), Math.round(position.getY(i) / unit), Math.round(position.getZ(i) / unit));
    const bucket = cells.get(cell) ?? [];
    if (bucket.length) join(i, bucket[0]);
    bucket.push(i); cells.set(cell, bucket);
  }
  for (const mesh of meshes) {
    const triangles = mesh.geometry.index;
    if (!triangles) continue;
    for (let i = 0; i < triangles.count; i += 3) {
      join(triangles.getX(i), triangles.getX(i + 1)); join(triangles.getX(i), triangles.getX(i + 2));
    }
  }
  const seeds = new Set<number>();
  const allowed = (i: number) => [0, 1, 2, 3].every(k => weights.getComponent(i, k) < .001
    || skeleton.bones[indices.getComponent(i, k)]?.name === 'J_Bip_C_Head'
    || chain.has(skeleton.bones[indices.getComponent(i, k)]));
  for (let i = 0; i < count; i++) {
    if (position.getX(i) > 0 && [0, 1, 2, 3].some(k => weights.getComponent(i, k) > .001
      && chain.has(skeleton.bones[indices.getComponent(i, k)]))) seeds.add(find(i));
  }
  if (!seeds.size) return null;
  const source: number[] = [], mirror = new Set<number>();
  for (let i = 0; i < count; i++) {
    if (!seeds.has(find(i)) || position.getX(i) <= 0 || !allowed(i)) continue;
    source.push(i);
    const x = -position.getX(i), y = position.getY(i), z = position.getZ(i);
    const qx = Math.round(x / unit), qy = Math.round(y / unit), qz = Math.round(z / unit);
    for (let dx = -1; dx <= 1; dx++) for (let dy = -1; dy <= 1; dy++) for (let dz = -1; dz <= 1; dz++) {
      for (const candidate of cells.get(key(qx + dx, qy + dy, qz + dz)) ?? []) {
        if (allowed(candidate) && (position.getX(candidate) - x) ** 2 + (position.getY(candidate) - y) ** 2 + (position.getZ(candidate) - z) ** 2 < unit ** 2 * 4) mirror.add(candidate);
      }
    }
  }
  return { meshes, position, indices, weights, source, mirror: [...mirror] };
}

function strandPath(surface: HairSurface, count: number) {
  const points = surface.source.map(i => new THREE.Vector3().fromBufferAttribute(surface.position, i));
  const bounds = new THREE.Box3().setFromPoints(points), height = bounds.max.y - bounds.min.y;
  const top = points.filter(p => p.y > bounds.max.y - height * .12);
  const innerX = Math.min(...top.map(p => p.x));
  const mean = (samples: THREE.Vector3[]) => samples.reduce((sum, p) => sum.add(p), new THREE.Vector3()).multiplyScalar(1 / samples.length);
  const controls = [mean(top.filter(p => p.x < innerX + height * .025))];
  for (let i = 0; i < 10; i++) {
    const y = bounds.max.y - height * (i + .5) / 10;
    const band = points.filter(p => Math.abs(p.y - y) < height / 20);
    if (band.length) controls.push(mean(band));
  }
  controls.push(mean(points.filter(p => p.y < bounds.min.y + height * .025)));
  return new THREE.CatmullRomCurve3(controls, false, 'centripetal').getSpacedPoints(count - 1);
}

function bindStrand(surface: HairSurface, vertices: number[], chain: THREE.Bone[], path: THREE.Vector3[], mirrored: boolean) {
  const skeleton = surface.meshes[0].skeleton, boneIndices = chain.map(b => skeleton.bones.indexOf(b));
  const headIndex = skeleton.bones.findIndex(b => b.name === 'J_Bip_C_Head');
  const point = new THREE.Vector3(), edge = new THREE.Vector3(), projected = new THREE.Vector3();
  const span = path.reduce((sum, p, i) => sum + (i ? p.distanceTo(path[i - 1]) : 0), 0) / (path.length - 1);
  const distances = new Float64Array(path.length - 1), depths = new Float64Array(path.length - 1);
  for (const i of vertices) {
    point.fromBufferAttribute(surface.position, i); if (mirrored) point.x *= -1;
    let nearest = Infinity, depth = 0;
    for (let j = 0; j < path.length - 1; j++) {
      edge.subVectors(path[j + 1], path[j]);
      const t = THREE.MathUtils.clamp(projected.subVectors(point, path[j]).dot(edge) / edge.lengthSq(), 0, 1);
      projected.copy(path[j]).addScaledVector(edge, t);
      const distance = projected.distanceToSquared(point);
      distances[j] = distance; depths[j] = j + t;
      nearest = Math.min(nearest, distance);
    }
    // At the curled crown a vertex can be equally close to two nonadjacent
    // segments. A hard nearest-segment choice tears neighboring triangles.
    // Blend projections continuously across that region instead.
    let total = 0;
    for (let j = 0; j < distances.length; j++) {
      const weight = Math.exp(-(distances[j] - nearest) / (span * span * .64));
      depth += depths[j] * weight; total += weight;
    }
    depth /= total;
    const joint = Math.min(chain.length - 2, Math.floor(depth)), blend = depth - joint;
    // Blend only at the scalp attachment; the rest uses adjacent moving joints.
    const moving = THREE.MathUtils.smoothstep(depth, 0, .65)
      * THREE.MathUtils.smoothstep(point.distanceTo(path[0]), 0, span * .45);
    surface.indices.setXYZW(i, headIndex, boneIndices[joint], boneIndices[joint + 1], 0);
    surface.weights.setXYZW(i, 1 - moving, (1 - blend) * moving, blend * moving, 0);
  }
  surface.indices.needsUpdate = true; surface.weights.needsUpdate = true;
}

// Fix the supplied mirrored VRM0 rig on its runtime copy, before optimization.
// Full ribbons are selected by connectivity, not just existing spring weights.
export function repairPet3dRig(vrm: VRM, model: number) {
  const suffix = model === 0 ? '10' : model === 2 ? '13' : null, manager = vrm.springBoneManager;
  if (!suffix || !manager || vrm.scene.userData.mofuRigRepaired) return;
  const joints = [...manager.joints].filter(j => new RegExp(`^J_Sec_Hair\\d+_${suffix}$`).test(j.bone.name));
  const root = joints.find(j => j.bone.name === `J_Sec_Hair1_${suffix}`)?.bone;
  if (!root?.parent || root.position.x >= 0) return;
  const head = root.parent, chain: THREE.Bone[] = [];
  root.traverse(node => { if (node instanceof THREE.Bone) chain.push(node); });
  const chainSet = new Set(chain), meshes: THREE.SkinnedMesh[] = [];
  vrm.scene.traverse(node => { if (node instanceof THREE.SkinnedMesh && node.skeleton.bones.includes(root as THREE.Bone)) meshes.push(node); });
  const groups = new Map<Attribute, THREE.SkinnedMesh[]>();
  for (const mesh of meshes) {
    const position = mesh.geometry.getAttribute('position'), group = groups.get(position) ?? [];
    group.push(mesh); groups.set(position, group);
  }
  const surfaces = [...groups.values()].map(group => findHairSurface(group, chainSet)).filter((s): s is HairSurface => !!s);
  if (!surfaces.length) return;
  const path = strandPath(surfaces[0], chain.length);
  const clones = chain.map(source => { const bone = new THREE.Bone(); bone.name = `${source.name}_MofuMirror`; return bone; });
  vrm.scene.updateMatrixWorld(true);
  const meshToHead = head.matrixWorld.clone().invert().multiply(surfaces[0].meshes[0].bindMatrix);
  for (const [side, bones] of [chain, clones].entries()) {
    const local = path.map(p => new THREE.Vector3(p.x * (side === 0 ? 1 : -1), p.y, p.z).applyMatrix4(meshToHead));
    bones.forEach((bone, i) => {
      (i ? bones[i - 1] : head).add(bone);
      bone.position.copy(local[i]); if (i) bone.position.sub(local[i - 1]);
      bone.quaternion.identity(); bone.scale.setScalar(1); bone.updateMatrix();
    });
  }
  vrm.scene.updateMatrixWorld(true);
  const skeletons = new Map<THREE.Skeleton, THREE.Skeleton>();
  for (const mesh of meshes) {
    const old = mesh.skeleton;
    let next = skeletons.get(old);
    if (!next) {
      next = new THREE.Skeleton([...old.bones, ...clones], [
        ...old.boneInverses.map((inverse, i) => chainSet.has(old.bones[i]) ? old.bones[i].matrixWorld.clone().invert() : inverse.clone()),
        ...clones.map(bone => bone.matrixWorld.clone().invert()),
      ]);
      skeletons.set(old, next);
    }
    mesh.skeleton = next;
  }
  for (const surface of surfaces) {
    bindStrand(surface, surface.source, chain, path, false);
    bindStrand(surface, surface.mirror, clones, path, true);
  }
  for (const joint of joints) manager.deleteJoint(joint);
  for (const bones of [chain, clones]) for (let i = 0; i < bones.length - 1; i++) {
    manager.addJoint(new SoftHairJoint(joints[0], bones[i], bones[i + 1], head, i));
  }
  if (model === 2) {
    // Preserve the long back hair's rig, preventing its oversized colliders from
    // folding strands through the face during a nod or head tilt.
    for (const joint of [...manager.joints]) {
      const match = /^J_Sec_Hair(\d+)_(08|09|10)$/.exec(joint.bone.name);
      if (!match) continue;
      manager.deleteJoint(joint);
      manager.addJoint(new SoftHairJoint(joint, joint.bone, joint.child, head, Number(match[1]) - 1, false));
    }
  }
  manager.setInitState();
  for (const old of skeletons.keys()) old.dispose();
  vrm.scene.userData.mofuRigRepaired = {
    sourceVertices: surfaces.reduce((sum, s) => sum + s.source.length, 0),
    mirroredVertices: surfaces.reduce((sum, s) => sum + s.mirror.length, 0),
    bonesAdded: chain.length, fullStrandBinding: true,
  };
}
