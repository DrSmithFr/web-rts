import * as THREE from 'three';

// Day/night cycle: a pure function of the simulation tick (see
// sim/day/day.go — constants mirrored, canonical in Go). The cycle
// rotates the sun direction, fades to a pale moon at night and
// crossfades sky, fog, hemisphere fill and exposure.

// DAY_TICKS mirrors day.DayTicks: one full cycle at 20 Hz (8 minutes).
export const DAY_TICKS = 9600;
// DAWN_OFFSET mirrors day.DawnOffset: tick 0 is early morning.
export const DAWN_OFFSET = 2880;

export function phaseOf(tick: number): number {
  return ((((tick + DAWN_OFFSET) % DAY_TICKS) + DAY_TICKS) % DAY_TICKS) / DAY_TICKS;
}

export function sunElevation(tick: number): number {
  return Math.sin((phaseOf(tick) - 0.25) * 2 * Math.PI);
}

const smoothstep = (a: number, b: number, x: number): number => {
  const t = Math.min(1, Math.max(0, (x - a) / (b - a)));
  return t * t * (3 - 2 * t);
};

const DAY_STOPS = ['#5f93cf', '#a8c8e8', '#e2d9b8', '#f0e3c2'];
const NIGHT_STOPS = ['#0b1020', '#141a30', '#242c48', '#2c3350'];
const WARM = '#f2a35f';

const col = (hex: string): THREE.Color => new THREE.Color(hex);
const tmp = [col(''), col(''), col(''), col('')];
const dayCols = DAY_STOPS.map(col);
const nightCols = NIGHT_STOPS.map(col);
const warmCol = col(WARM);

export class Daylight {
  readonly dir = new THREE.Vector3(0.62, 1, 0.45).normalize();
  private lastTick = -1;
  private nightVal = 0;
  private readonly sunColor = new THREE.Color();
  private readonly warmLow = col('#ff9a5a');
  private readonly highSun = col('#fff0d8');
  private readonly moon = col('#9db8ff');
  private readonly hemiDay = { sky: col('#cfe6ff'), ground: col('#46512e') };
  private readonly hemiNight = { sky: col('#2a3a55'), ground: col('#151b10') };
  private readonly fogDay = col('#e8ddc2');
  private readonly fogNight = col('#262e46');

  constructor(
    private readonly sun: THREE.DirectionalLight,
    private readonly hemi: THREE.HemisphereLight,
    private readonly fog: THREE.Fog,
    private readonly sky: THREE.CanvasTexture,
    private readonly renderer: THREE.WebGLRenderer,
  ) {
    tmp.forEach((c, i) => c.copy(dayCols[i]));
  }

  // night is 1 at midnight, 0 in full day — campfire glow and future
  // night-aware visuals blend on this.
  get night(): number {
    return this.nightVal;
  }

  // update advances the cycle for the given (possibly repeating) tick
  // and repaints the sky canvas only when the tick actually moved.
  update(tick: number): void {
    if (tick === this.lastTick) return;
    this.lastTick = tick;

    const elev = sunElevation(tick);
    const day = smoothstep(-0.08, 0.22, elev);
    this.nightVal = 1 - day;
    const warm = Math.max(0, 1 - Math.abs(elev) * 3.2) * (elev > -0.3 ? 1 : 0);

    // light direction: sun on a great circle; below the horizon it
    // becomes the moon, opposite side, still lighting from above.
    const az = phaseOf(tick) * 2 * Math.PI;
    const horiz = Math.sqrt(Math.max(0, 1 - elev * elev));
    const y = Math.max(Math.abs(elev), 0.3);
    this.dir.set(Math.cos(az) * horiz, y, Math.sin(az) * horiz);
    if (elev < 0) {
      this.dir.x *= -1;
      this.dir.z *= -1;
    }
    this.dir.normalize();

    this.sun.intensity = day > 0.02 ? 2.1 * day : 0.85;
    if (elev < 0) {
      this.sun.color.copy(this.moon);
    } else {
      this.sunColor.copy(this.warmLow).lerp(this.highSun, smoothstep(0.0, 0.45, elev));
      this.sun.color.copy(this.sunColor);
    }

    (this.hemi.color as THREE.Color).copy(this.hemiNight.sky).lerp(this.hemiDay.sky, day);
    (this.hemi.groundColor as THREE.Color).copy(this.hemiNight.ground).lerp(this.hemiDay.ground, day);
    this.hemi.intensity = 0.45 + 0.5 * day;

    this.fog.color.copy(this.fogNight).lerp(this.fogDay, day);
    this.renderer.toneMappingExposure = 0.98 + 0.14 * day;

    for (let i = 0; i < 4; i++) {
      tmp[i].copy(nightCols[i]).lerp(dayCols[i], day);
      if (i >= 2) tmp[i].lerp(warmCol, warm * 0.55);
    }
    this.paintSky();
  }

  private paintSky(): void {
    const canvas = this.sky.image as HTMLCanvasElement;
    const ctx = canvas.getContext('2d')!;
    const g = ctx.createLinearGradient(0, 0, 0, canvas.height);
    g.addColorStop(0, `#${tmp[0].getHexString()}`);
    g.addColorStop(0.55, `#${tmp[1].getHexString()}`);
    g.addColorStop(0.85, `#${tmp[2].getHexString()}`);
    g.addColorStop(1, `#${tmp[3].getHexString()}`);
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    this.sky.needsUpdate = true;
  }
}
