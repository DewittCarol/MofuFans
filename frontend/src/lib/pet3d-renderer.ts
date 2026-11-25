import * as THREE from 'three';
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';
import { VRMLoaderPlugin, VRMUtils, type VRM, type VRMHumanBoneName } from '@pixiv/three-vrm';
import { pet3dGaze, pet3dModels, pet3dMotion, pet3dActions, pet3dExpressions, pet3dExpressionWeights, resolvePet3dExpression } from './pet3d-model';
import { configurePet3dPhysics, pet3dSimulationSteps } from './pet3d-physics';
import { repairPet3dRig } from './pet3d-rig';
import type { PetEngine } from './pet-engine';

export type Pet3dModelDefinition = { family: number; name: string; url: string };
type LoadOptions = { signal: AbortSignal; model: number; definition?: Pet3dModelDefinition; onProgress: (progress: number) => void; onError: () => void };

async function modelBytes(url: string, { signal, onProgress }: LoadOptions) {
  const response = await fetch(url, { signal });
  if (!response.ok) throw new Error('3D model unavailable');
  const total = Number(response.headers.get('Content-Length'));
  if (!response.body || !total) return response.arrayBuffer();
  const reader = response.body.getReader(), parts: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      signal.throwIfAborted();
      const { done, value } = await reader.read();
      if (done) break;
      parts.push(value); size += value.byteLength;
      onProgress(Math.min(95, Math.round(size / total * 95)));
    }
  } finally { reader.releaseLock(); }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const part of parts) { bytes.set(part, offset); offset += part.byteLength; }
  return bytes.buffer;
}

export class Pet3dRenderer implements PetEngine {
  readonly aspectRatio = .7;
  private scene = new THREE.Scene();
  private camera = new THREE.PerspectiveCamera(28, this.aspectRatio, .01, 100);
  private lookTarget = new THREE.Object3D();
  private headPosition = new THREE.Vector3();
  private bones = new Map<VRMHumanBoneName, THREE.Object3D>();
  private expressions: string[];
  private height = 1;
  private frame = 0;
  private previous = 0;
  private clock = 0;
  private nextBlink = 3;
  private blinkUntil = 0;
  private responseUntil = 0;
  private action: string | null = null;
  private actionStarted = 0;
  private expression = 0;
  private selectedExpression = 'neutral';
  private reactionFace: string | null = null;
  private responseTimer = 0;
  private active = false;
  private paused = false;
  private reduced = false;
  private disposed = false;
  private pointer = { x: 0, y: 0 };
  private target = { x: 0, y: 0 };
  private frames = 0;

  static async load(canvas: HTMLCanvasElement, options: LoadOptions) {
    const { signal } = options;
    const model = options.definition ?? pet3dModels[options.model] ?? pet3dModels[0];
    if (!model?.url) throw new Error('请先配置桌宠模型地址');
    const bytes = await modelBytes(model.url, options);
    signal.throwIfAborted();
    const loader = new GLTFLoader();
    loader.register(parser => new VRMLoaderPlugin(parser));
    const gltf = await loader.parseAsync(bytes, model.url.slice(0, model.url.lastIndexOf('/') + 1));
    const vrm = gltf.userData.vrm as VRM | undefined;
    if (!vrm) { VRMUtils.deepDispose(gltf.scene); throw new Error('Invalid VRM'); }
    let renderer: THREE.WebGLRenderer | undefined;
    let pet: Pet3dRenderer | undefined;
    try {
      signal.throwIfAborted();
      if (model.family === 1) repairPet3dRig(vrm, options.model);
      VRMUtils.rotateVRM0(vrm);
      VRMUtils.removeUnnecessaryVertices(vrm.scene);
      VRMUtils.combineSkeletons(vrm.scene);
      renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: 'low-power' });
      pet = new Pet3dRenderer(canvas, renderer, vrm, options.onError, model.family === 1);
      await renderer.compileAsync(pet.scene, pet.camera);
      signal.throwIfAborted();
      canvas.dataset.model = String(options.model);
      options.onProgress(100);
      return pet;
    } catch (error) {
      if (pet) pet.dispose();
      else { VRMUtils.deepDispose(vrm.scene); renderer?.dispose(); renderer?.forceContextLoss(); }
      throw error;
    }
  }

  private constructor(private canvas: HTMLCanvasElement, private renderer: THREE.WebGLRenderer, private vrm: VRM, private onError: () => void, originalRig: boolean) {
    renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 1.5));
    renderer.setClearColor(0x000000, 0);
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.scene.add(new THREE.HemisphereLight(0xffffff, 0xddb9c6, .7));
    const light = new THREE.DirectionalLight(0xffffff, 2);
    light.position.set(1, 2, 3); this.scene.add(light, this.lookTarget, vrm.scene);
    vrm.scene.traverse(object => { object.frustumCulled = false; });
    const box = new THREE.Box3().setFromObject(vrm.scene);
    this.height = box.max.y - box.min.y; vrm.scene.position.y = -box.min.y;
    if (originalRig) configurePet3dPhysics(vrm, this.height);
    this.camera.position.set(0, this.height * .52, this.height * 2.25);
    this.camera.lookAt(0, this.height * .52, 0);
    if (vrm.lookAt) vrm.lookAt.target = this.lookTarget;
    for (const name of ['leftUpperArm', 'rightUpperArm', 'rightLowerArm', 'rightHand', 'chest', 'head'] as VRMHumanBoneName[]) {
      const bone = vrm.humanoid.getNormalizedBoneNode(name);
      if (bone) this.bones.set(name, bone);
    }
    this.expressions = Object.keys(vrm.expressionManager?.expressionMap ?? {});
    canvas.addEventListener('webglcontextlost', this.contextLost);
    this.resize();
  }

  private contextLost = (event: Event) => {
    event.preventDefault();
    if (this.disposed) return;
    this.dispose(); this.onError();
  };

  resize() {
    if (this.disposed) return;
    const { width, height } = this.canvas.getBoundingClientRect();
    if (!width || !height) return;
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height; this.camera.updateProjectionMatrix(); this.draw();
  }
  setPointer(x: number, y: number) { this.target = { x, y }; }
  setExpression(expression: string) {
    if (this.disposed) return;
    this.selectedExpression = pet3dExpressions.some(item => item.id === expression) ? expression : 'neutral';
    this.clearResponse(); this.draw();
  }
  playAction(action: string) {
    if (this.disposed || !this.active || this.paused || !pet3dActions.some(item => item.id === action)) return;
    if (this.reduced) { this.respond(1); return; }
    this.action = action; this.actionStarted = this.clock;
  }
  reactToEmoji(face: string, action: string) {
    if (this.disposed || !this.active) return;
    this.clearResponse(); this.reactionFace = face; this.responseUntil = this.clock+3.2;
    if (!this.paused && !this.reduced) this.playAction(action);
    this.draw();
    if (this.paused || this.reduced) this.responseTimer = window.setTimeout(() => { this.clearResponse(); this.draw(); }, 3200);
  }
  setActive(active: boolean) {
    if (this.disposed || this.active === active) return;
    this.active = active;
    if (!active) { this.clearResponse(); this.action = null; }
    this.sync();
    if (active) { this.pose(); this.vrm.springBoneManager?.reset(); this.render(); }
  }
  setPreferences(paused: boolean, reduced: boolean) {
    if (this.disposed) return;
    this.paused = paused; this.reduced = reduced;
    this.pointer = { x: 0, y: 0 }; this.blinkUntil = 0;
    this.action = null;
    this.clearResponse(); this.sync(); this.pose(); this.vrm.springBoneManager?.reset(); this.render();
  }
  respond(expression = 2) {
    if (this.disposed || !this.active) return;
    this.clearResponse(); this.expression = expression;
    this.responseUntil = this.clock + 2.3;
    if (expression === 2 && !this.paused && !this.reduced) this.playAction('wave');
    this.draw();
    if (this.paused || this.reduced) {
      this.responseTimer = window.setTimeout(() => { this.clearResponse(); this.draw(); }, 2300);
    }
  }
  private clearResponse() {
    window.clearTimeout(this.responseTimer); this.responseTimer = 0;
    this.expression = 0; this.reactionFace = null; this.responseUntil = 0;
  }
  private sync() {
    if (this.frame) cancelAnimationFrame(this.frame);
    this.frame = 0; this.previous = 0;
    const moving = this.active && !this.paused && !this.reduced && !this.disposed;
    this.canvas.dataset.animating = String(moving);
    if (moving) this.frame = requestAnimationFrame(this.tick);
  }
  private tick = (now: number) => {
    this.frame = 0;
    if (this.disposed || !this.active || this.paused || this.reduced) return;
    const elapsed = now - this.previous;
    if (!this.previous || elapsed >= 1000 / 30 - .5) {
      const steps = pet3dSimulationSteps(this.previous ? elapsed / 1000 : 1 / 30);
      this.previous = now;
      for (let i = 0; i < steps.count; i++) {
        this.clock += steps.delta;
        const ease = 1 - Math.exp(-steps.delta * 6);
        this.pointer.x += (this.target.x - this.pointer.x) * ease;
        this.pointer.y += (this.target.y - this.pointer.y) * ease;
        if (this.clock > this.nextBlink) { this.blinkUntil = this.clock + .14; this.nextBlink = this.clock + 3 + Math.random() * 3; }
        if (this.responseUntil && this.clock >= this.responseUntil) this.clearResponse();
        this.pose(steps.delta);
      }
      this.render();
    }
    this.frame = requestAnimationFrame(this.tick);
  };

  private pose(dt = 0) {
    if (this.disposed) return;
    const moving = !this.paused && !this.reduced;
    const time = moving ? this.clock : 0;
    const actionTime = this.clock - this.actionStarted;
    const motion = pet3dMotion(moving ? this.action : null, actionTime);
    if (this.action && actionTime >= (pet3dActions.find(item => item.id === this.action)?.duration ?? 0)) this.action = null;
    const sway = moving ? Math.sin(time * 1.3) * .012 : 0;
    this.bones.get('leftUpperArm')?.rotation.set(0, 0, 1.13 + sway);
    // Bring the upper arm forward and share the outward turn at the shoulder.
    // The elbow stays lower, with a modest forearm twist rather than a hard seam.
    this.bones.get('rightUpperArm')?.rotation.set(motion.upperArmRoll, motion.upperArmForward, -1.13 + motion.upperArm - sway, 'ZYX');
    this.bones.get('rightLowerArm')?.rotation.set(motion.forearmRoll, 0, motion.lowerArm, 'ZYX');
    // Local Y is the palm normal; keep the small wave in the palm's plane.
    this.bones.get('rightHand')?.rotation.set(motion.handRoll, motion.hand, 0, 'XYZ');
    this.bones.get('chest')?.rotation.set(sway * .3, sway * .4, sway * .35);
    const gaze = pet3dGaze(moving ? this.pointer.x : 0, moving ? this.pointer.y : 0);
    const followWeight = 1-motion.weight*.65;
    this.bones.get('head')?.rotation.set(gaze.pitch*followWeight+motion.pitch, gaze.yaw*followWeight+motion.yaw, motion.roll);
    this.vrm.humanoid.update(); this.vrm.scene.updateMatrixWorld(true);
    this.vrm.humanoid.getRawBoneNode('head')?.getWorldPosition(this.headPosition);
    this.lookTarget.position.set(this.headPosition.x + gaze.eyeX * this.height,
      this.headPosition.y + gaze.eyeY * this.height, this.headPosition.z + this.height * 2);
    const ex = this.vrm.expressionManager;
    for (const name of this.expressions) ex?.setValue(name, 0);
    const face = this.reactionFace ?? (this.expression ? (this.expression === 1 ? 'relaxed' : 'happy') : this.selectedExpression);
    for (const [name, value] of Object.entries(pet3dExpressionWeights(face))) {
      const key = resolvePet3dExpression(this.expressions, name);
      if (key !== 'neutral') ex?.setValue(key, value);
    }
    ex?.setValue('blink', moving && this.clock < this.blinkUntil ? 1 : 0);
    this.vrm.update(moving ? dt : 0);
    this.canvas.dataset.gazePitch = gaze.pitch.toFixed(3);
    this.canvas.dataset.eyeY = gaze.eyeY.toFixed(3);
    this.canvas.dataset.expression = face;
    this.canvas.dataset.action = this.action ?? 'idle';
    this.canvas.dataset.actionTime = actionTime.toFixed(2);
  }

  private render() {
    if (this.disposed) return;
    this.renderer.render(this.scene, this.camera);
    this.canvas.dataset.frames = String(++this.frames);
  }

  private draw() {
    if (this.disposed) return;
    this.pose(); this.render();
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true; this.active = false;
    this.clearResponse(); this.sync();
    this.canvas.removeEventListener('webglcontextlost', this.contextLost);
    const bitmaps = new Set<ImageBitmap>();
    this.vrm.scene.traverse(object => {
      if (!(object instanceof THREE.Mesh)) return;
      for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
        for (const value of Object.values(material)) {
          if (value instanceof THREE.Texture && typeof ImageBitmap !== 'undefined' && value.image instanceof ImageBitmap) bitmaps.add(value.image);
        }
      }
    });
    VRMUtils.deepDispose(this.vrm.scene); bitmaps.forEach(bitmap => bitmap.close());
    this.scene.clear(); this.renderer.dispose(); this.renderer.forceContextLoss();
    this.canvas.width = 1; this.canvas.height = 1;
  }
}
