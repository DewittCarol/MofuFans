// Shared lifecycle so switching between 2D and 3D releases the previous renderer.
export interface PetEngine {
  readonly aspectRatio: number;
  resize(): void;
  setPointer(x: number, y: number): void;
  setScene?(scene: number): void;
  setExpression?(expression: string): void;
  playAction?(action: string): void;
  reactToEmoji?(face: string, action: string): void;
  setPreferences(paused: boolean, reduced: boolean): void;
  setActive(active: boolean): void;
  respond(expression?: number): void;
  dispose(): void;
}
