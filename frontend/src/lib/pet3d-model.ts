// Models belong to each deployment. No character model is distributed with the app.
export const pet3dModels: {family:number;name:string;url:string}[] = [];

export const pet3dExpressions = [
  { id: 'neutral', name: '平常' },
  { id: 'happy', name: '开心' },
  { id: 'angry', name: '生气' },
  { id: 'sad', name: '委屈' },
  { id: 'surprised', name: '惊讶' },
  { id: 'relaxed', name: '放松' },
  { id: 'curious', name: '疑惑' },
  { id: 'teasing', name: '调皮' },
] as const;
export type Pet3dExpression = typeof pet3dExpressions[number]['id'];

export const pet3dActions = [
  { id: 'wave', name: '挥手', duration: 2.4 },
  { id: 'nod', name: '点头', duration: 2 },
  { id: 'shake', name: '摇头', duration: 2.2 },
  { id: 'tilt', name: '歪头', duration: 2.5 },
  { id: 'giggle', name: '偷笑', duration: 2.4 },
  { id: 'comfort', name: '低头', duration: 2.8 },
] as const;
export type Pet3dAction = typeof pet3dActions[number]['id'];

export function pet3dMotion(action: string | null, time: number) {
  const clip = pet3dActions.find(item => item.id === action);
  const rest = { weight: 0, pitch: 0, yaw: 0, roll: 0, upperArm: 0, upperArmForward: 0, upperArmRoll: 0, lowerArm: 0, forearmRoll: 0, handRoll: 0, hand: 0 };
  if (!clip || !Number.isFinite(time) || time <= 0 || time >= clip.duration) return rest;
  const smooth = (x: number) => { const t = Math.min(1, Math.max(0, x)); return t*t*(3-2*t); };
  const weight = smooth(time/.35)*smooth((clip.duration-time)/.4);
  if (action === 'nod') return { ...rest, weight, pitch: -Math.sin(time*Math.PI*2*1.25)*.26*weight };
  if (action === 'shake') return { ...rest, weight, yaw: Math.sin(time*Math.PI*2*1.35)*.32*weight };
  if (action === 'tilt') return { ...rest, weight, roll: .2*weight, yaw: -.06*weight };
  if (action === 'giggle') return { ...rest, weight, pitch: -.1*Math.abs(Math.sin(time*8))*weight, roll: Math.sin(time*4)*.04*weight };
  if (action === 'comfort') return { ...rest, weight, pitch: -.18*weight, roll: .06*weight };
  const beat=time*7.2;
  // Let the shoulder carry the gesture. Distribute the palm turn through the
  // upper arm, forearm and wrist instead of twisting 90 degrees at the elbow.
  return { ...rest, weight, upperArm: weight*(1.03+Math.sin(beat)*.035),
    upperArmForward: weight*(.55+Math.sin(beat)*.02), upperArmRoll: weight*.6,
    lowerArm: weight*(1.25+Math.sin(beat+.4)*.055), forearmRoll: weight*.65,
    handRoll: weight*.1, hand: weight*Math.sin(beat)*.16 };
}

export function pet3dExpressionWeights(face: string): Record<string, number> {
  if (face === 'neutral') return {};
  if (face === 'curious') return { surprised: .4, sad: .12, oh: .16 };
  if (face === 'teasing') return { happy: .72, relaxed: .28, aa: .18 };
  return { [face]: .85 };
}

export function resolvePet3dExpression(available: readonly string[], requested: string) {
  return available.find(name => name.toLowerCase() === requested.toLowerCase()) ?? 'neutral';
}

export function pet3dGaze(x: number, y: number) {
  const horizontal = Math.min(1, Math.max(-1, Number.isFinite(x) ? x : 0));
  const vertical = Math.min(1, Math.max(-1, Number.isFinite(y) ? y : 0));
  // Normalized VRM bones face -Z: negative X rotation looks DOWN.
  // Pointer Y increases down the screen, whereas world Y increases upward.
  return { yaw: horizontal * .42, pitch: -vertical * .24, eyeX: horizontal * 1, eyeY: -vertical * .85 };
}
