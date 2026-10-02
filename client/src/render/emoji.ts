import * as THREE from 'three';

// Condition glyphs shown above an entity. The order is the priority: the
// first entry whose bit is set wins, so terror outranks rage and fear.
// Bits mirror sim/core/condition.go (CondFear 1, CondTerror 2, CondRage 4).
export const CONDITION_GLYPHS = [
  { bit: 2, glyph: '😱' }, // terror
  { bit: 4, glyph: '😡' }, // rage
  { bit: 1, glyph: '😨' }, // fear
];

// conditionGlyph is the slot in CONDITION_GLYPHS for a condition bitmask,
// or -1 when the entity carries no condition worth showing.
export function conditionGlyph(cond: number): number {
  return CONDITION_GLYPHS.findIndex((c) => (cond & c.bit) !== 0);
}

// emojiTexture rasterises one emoji glyph on a transparent card so it can
// sit on an instanced quad above an entity. Built once at startup.
export function emojiTexture(glyph: string): THREE.CanvasTexture {
  const size = 96;
  const canvas = document.createElement('canvas');
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext('2d')!;
  ctx.clearRect(0, 0, size, size);
  ctx.font = `${size - 16}px "Noto Color Emoji", "Apple Color Emoji", "Segoe UI Emoji", sans-serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(glyph, size / 2, size / 2);
  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  return texture;
}
