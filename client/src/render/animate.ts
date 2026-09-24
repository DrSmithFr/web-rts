// Pure animation math for the rigid-part actors. Everything here is
// integer/float math on client-observed state (interpolated positions,
// unit state bytes); the simulation is never asked for animation values.

// angDiff returns the shortest signed turn from a to b, in radians.
export function angDiff(a: number, b: number): number {
  let d = b - a;
  while (d > Math.PI) d -= Math.PI * 2;
  while (d < -Math.PI) d += Math.PI * 2;
  return d;
}

// stepYaw turns yaw towards target by at most rate*dt radians.
export function stepYaw(yaw: number, target: number, dt: number, rate = 12): number {
  if (!Number.isFinite(target)) return yaw;
  const d = angDiff(yaw, target);
  const max = rate * dt;
  if (Math.abs(d) <= max) return wrap(target);
  return wrap(yaw + Math.sign(d) * max);
}

// walkPhase advances by two full steps per tile travelled.
export function walkPhase(phase: number, tilesMoved: number): number {
  const p = phase + tilesMoved * Math.PI * 2;
  return p % (Math.PI * 2048);
}

// walkAmp returns the leg swing amplitude for a ground speed, eased.
export function walkAmp(current: number, speed: number, dt: number): number {
  const target = speed < 0.05 ? 0 : Math.min(0.55, speed * 0.4);
  const k = Math.min(1, dt * 8);
  return current + (target - current) * k;
}

function wrap(a: number): number {
  let m = (a + Math.PI) % (Math.PI * 2);
  if (m < 0) m += Math.PI * 2;
  return m - Math.PI;
}
