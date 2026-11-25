// A small web format exported from the supplied artwork, independent of its editor.
export type PetPose = {
  x: number; y: number; ox: number; oy: number; sx: number; sy: number;
  rotation: number; z: number; visible: boolean;
  blink: boolean | null; talk: boolean | null;
  sway: [number, number, number, number]; follow: [number, number]; drag: number;
  clip?: number; frames?: number; fps?: number; stillFrame?: number | null;
};
export type PetLayer = {
  parent: number | null; name: string; image?: string; width?: number; height?: number;
  poses: PetPose[]; states: number[];
  drawRect?: [number, number, number, number];
  appendage?: { segments: number; maxAngle: number };
};
export type PetModel = {
  version: number; name: string; viewBox: [number, number, number, number];
  stateCount: number; layers: PetLayer[];
};
export type Matrix = [number, number, number, number, number, number];
export const identity: Matrix = [1, 0, 0, 1, 0, 0];
export const clamp = (value: number, min = 0, max = 1) => Math.min(max, Math.max(min, value));

export function multiply(a: Matrix, b: Matrix): Matrix {
  return [a[0]*b[0]+a[2]*b[1], a[1]*b[0]+a[3]*b[1],
    a[0]*b[2]+a[2]*b[3], a[1]*b[2]+a[3]*b[3],
    a[0]*b[4]+a[2]*b[5]+a[4], a[1]*b[4]+a[3]*b[5]+a[5]];
}

export function localMatrix(p: PetPose, dx = 0, dy = 0, angle = 0): Matrix {
  const r = p.rotation + angle, c = Math.cos(r), s = Math.sin(r);
  const a = c*p.sx, b = s*p.sx, cc = -s*p.sy, d = c*p.sy;
  // Child layers are attached to the parent's image origin, including its offset.
  return [a, b, cc, d, p.x+dx+a*p.ox+cc*p.oy, p.y+dy+b*p.ox+d*p.oy];
}

export function getPose(layer: PetLayer, state: number) {
  return layer.poses[layer.states[state] ?? layer.states[0]];
}

export function isLayerVisible(pose: PetPose, eyesOpen: boolean, mouthOpen: boolean) {
  return pose.visible && (pose.blink === null || pose.blink === eyesOpen)
    && (pose.talk === null || pose.talk === mouthOpen);
}

export function petSheetFrame(pose: PetPose, clock: number, moving: boolean) {
  const frames = Math.max(1, Math.floor(pose.frames ?? 1));
  if (pose.stillFrame != null) return clamp(Math.floor(pose.stillFrame), 0, frames - 1);
  return moving ? Math.floor(clock * clamp(pose.fps ?? 1, 0, 60)) % frames : 0;
}

export type PetMode = 'pngtuber' | '3d';
export type PetPreferences = { collapsed: boolean; paused: boolean; scene: number; x: number; y: number; mode: PetMode; model3d: number; size3d: number };
export function readPetPreferences(value: string | null, smallScreen: boolean): PetPreferences {
  const defaults: PetPreferences = { collapsed: smallScreen, paused: false, scene: 0, x: 0, y: 1, mode: '3d', model3d: 3, size3d: 50 };
  try {
    const data = JSON.parse(value || 'null');
    if (!data || typeof data !== 'object' || Array.isArray(data)) return defaults;
    return {
      collapsed: typeof data.collapsed === 'boolean' ? data.collapsed : defaults.collapsed,
      paused: typeof data.paused === 'boolean' ? data.paused : false,
      scene: Number.isInteger(data.scene) ? clamp(data.scene, 0, 3) : 0,
      x: typeof data.x === 'number' && Number.isFinite(data.x) ? clamp(data.x) : 0,
      y: typeof data.y === 'number' && Number.isFinite(data.y) ? clamp(data.y) : 1,
      mode: data.mode === '3d' || data.mode === 'pngtuber' ? data.mode : defaults.mode,
      size3d: typeof data.size3d === 'number' && Number.isFinite(data.size3d) ? clamp(data.size3d, 25, 100) : 50,
      model3d: Number.isInteger(data.model3d) ? clamp(data.model3d, 0, 6) : defaults.model3d,
    };
  } catch { return defaults; }
}

export function petPosition(x: number, y: number, viewportWidth: number, viewportHeight: number, width: number, height: number) {
  const minX = 12, maxX = Math.max(minX, viewportWidth-width-12);
  const minY = 64, maxY = Math.max(minY, viewportHeight-height-20);
  return { x: minX+clamp(x)*(maxX-minX), y: minY+clamp(y)*(maxY-minY), minX, maxX, minY, maxY };
}

export function preservePetControlAnchor(anchorX: number, anchorY: number, controlX: number, controlY: number,
  rootX: number, rootY: number, viewportWidth: number, viewportHeight: number, width: number, height: number) {
  const bounds = petPosition(0, 0, viewportWidth, viewportHeight, width, height);
  const x = clamp(rootX + anchorX - controlX, bounds.minX, bounds.maxX);
  const y = clamp(rootY + anchorY - controlY, bounds.minY, bounds.maxY);
  const rangeX = Math.max(1, bounds.maxX - bounds.minX);
  const rangeY = Math.max(1, bounds.maxY - bounds.minY);
  return { ...bounds, x, y, normalizedX: clamp((x - bounds.minX) / rangeX), normalizedY: clamp((y - bounds.minY) / rangeY) };
}

export function notifyMofuPet(kind: 'question' | 'reaction', reaction?: { value: string; name?: string; url?: string }) {
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('mofu-pet-response', { detail: { kind, reaction } }));
}
