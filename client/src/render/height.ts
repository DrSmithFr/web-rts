import { HEIGHT_SCALE } from './palette';

// Bilinear ground-height sampler over the (size+1)² vertex height grid,
// matching the piecewise-linear terrain surface. World coordinates may be
// fractional; out-of-bounds is clamped to the map edge.
export function heightSampler(size: number, heights: Uint16Array): (x: number, z: number) => number {
  const corner = (tx: number, tz: number): number => heights[tz * (size + 1) + tx] * HEIGHT_SCALE;
  return (x: number, z: number): number => {
    const tx = Math.min(size - 1, Math.max(0, Math.floor(x)));
    const tz = Math.min(size - 1, Math.max(0, Math.floor(z)));
    const fx = Math.min(1, Math.max(0, x - tx));
    const fz = Math.min(1, Math.max(0, z - tz));
    const h00 = corner(tx, tz);
    const h10 = corner(tx + 1, tz);
    const h01 = corner(tx, tz + 1);
    const h11 = corner(tx + 1, tz + 1);
    return h00 + (h10 - h00) * fx + (h01 - h00) * fz + (h00 - h10 - h01 + h11) * fx * fz;
  };
}
