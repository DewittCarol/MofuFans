import { clamp, petPosition } from './pet-model';

export function preservePetControlAnchor(anchorX: number, anchorY: number, controlX: number, controlY: number,
  rootX: number, rootY: number, viewportWidth: number, viewportHeight: number, width: number, height: number) {
  const bounds = petPosition(0, 0, viewportWidth, viewportHeight, width, height);
  const x = clamp(rootX + anchorX - controlX, bounds.minX, bounds.maxX);
  const y = clamp(rootY + anchorY - controlY, bounds.minY, bounds.maxY);
  const rangeX = Math.max(1, bounds.maxX - bounds.minX);
  const rangeY = Math.max(1, bounds.maxY - bounds.minY);
  return { ...bounds, x, y, normalizedX: clamp((x - bounds.minX) / rangeX), normalizedY: clamp((y - bounds.minY) / rangeY) };
}
