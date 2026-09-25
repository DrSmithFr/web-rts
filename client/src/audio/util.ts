// Small shared helpers for the audio layer: persisted split volumes and
// a rate limiter so bursty game events cannot spam the synth.

export interface Volumes {
  music: number;
  sfx: number;
}

const MUSIC_KEY = 'rts.audio.music';
const SFX_KEY = 'rts.audio.sfx';
const DEFAULT_MUSIC = 70;
const DEFAULT_SFX = 80;

export function clampPct(v: number): number {
  if (!Number.isFinite(v)) return 0;
  return Math.max(0, Math.min(100, Math.round(v)));
}

export function loadVolumes(): Volumes {
  const read = (key: string, fallback: number): number => {
    try {
      const raw = localStorage.getItem(key);
      if (raw === null) return fallback;
      const v = Number(raw);
      return Number.isFinite(v) ? clampPct(v) : fallback;
    } catch {
      return fallback;
    }
  };
  return { music: read(MUSIC_KEY, DEFAULT_MUSIC), sfx: read(SFX_KEY, DEFAULT_SFX) };
}

export function saveVolumes(v: Volumes): void {
  try {
    localStorage.setItem(MUSIC_KEY, String(clampPct(v.music)));
    localStorage.setItem(SFX_KEY, String(clampPct(v.sfx)));
  } catch {
    // private mode or storage disabled: volumes stay session-only
  }
}

// Throttle allows at most one event per window.
export class Throttle {
  private last = -Infinity;

  constructor(private readonly ms: number) {}

  allow(now = Date.now()): boolean {
    if (now - this.last >= this.ms) {
      this.last = now;
      return true;
    }
    return false;
  }
}
