// Pure procedural music generation: seeded RNG, fixed scale per match,
// chord progression in 4/8-bar blocks, mutating 2-bar motifs, mood ->
// layer table and biome -> timbre presets. No audio API here, so every
// rule is unit-testable and replayable from a seed.

export type Mood = 'calm' | 'tension' | 'combat';

export type Rng = () => number;

// mulberry32: small deterministic PRNG so a seed reproduces a sequence.
export function mulberry32(seed: number): Rng {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// Natural minor scale, semitone offsets from the tonic.
export const MINOR_STEPS = [0, 2, 3, 5, 7, 8, 10] as const;

// One fixed tonic per match, derived from the seed (A2 .. G#3 range).
export function rootFromSeed(seed: number): number {
  return 45 + (Math.abs(Math.trunc(seed)) % 12);
}

// scaleMidi maps a scale-degree index (any integer, 7 per octave) to a
// MIDI note, so melodies and chords can never leave the scale.
export function scaleMidi(root: number, degree: number): number {
  const octave = Math.floor(degree / 7);
  const idx = degree - octave * 7;
  return root + MINOR_STEPS[idx] + 12 * octave;
}

// Degree walks for the progression: offsets mod 7 with musical weights.
// Every step is non-zero so a chord never repeats back to back.
const DEGREE_STEPS: { step: number; w: number }[] = [
  { step: 1, w: 3 },
  { step: 6, w: 2 },
  { step: 4, w: 2 },
  { step: 3, w: 2 },
  { step: 2, w: 1 },
  { step: 5, w: 1 },
];

export function nextDegree(rng: Rng, prev: number): number {
  const total = DEGREE_STEPS.reduce((s, d) => s + d.w, 0);
  let r = rng() * total;
  for (const d of DEGREE_STEPS) {
    r -= d.w;
    if (r < 0) return (prev + d.step) % 7;
  }
  return (prev + DEGREE_STEPS[0].step) % 7;
}

// One progression block of 4 or 8 bars, continuing from the last chord.
export function nextBlock(rng: Rng, prev: number): number[] {
  const len = rng() < 0.5 ? 4 : 8;
  const out: number[] = [];
  let d = prev;
  for (let i = 0; i < len; i++) {
    d = nextDegree(rng, d);
    out.push(d);
  }
  return out;
}

// Two-bar melody motif on the 8th-note grid (16 slots). Notes are scale
// degrees in [MELODY_LO, MELODY_HI]; null is a rest.
export const MOTIF_LEN = 16;
export const MELODY_LO = 2;
export const MELODY_HI = 13;

export interface Motif {
  notes: (number | null)[];
}

export function makeMotif(rng: Rng): Motif {
  const notes: (number | null)[] = [];
  let cur = MELODY_LO + 1 + Math.floor(rng() * (MELODY_HI - MELODY_LO - 1));
  let rests = 0;
  for (let i = 0; i < MOTIF_LEN; i++) {
    if (i > 0 && rests < 2 && rng() < 0.2) {
      notes.push(null);
      rests++;
      continue;
    }
    if (i > 0) {
      const r = rng();
      let move = 0;
      if (r < 0.55) move = rng() < 0.5 ? -1 : 1;
      else if (r < 0.78) move = rng() < 0.5 ? -2 : 2;
      else if (r < 0.94) move = rng() < 0.5 ? -3 : 3;
      else move = rng() < 0.5 ? -4 : 4;
      cur = Math.max(MELODY_LO, Math.min(MELODY_HI, cur + move));
    }
    notes.push(cur);
    rests = 0;
  }
  return { notes };
}

// Repeated iterations keep the motif but change one or two notes.
export function mutateMotif(rng: Rng, motif: Motif): Motif {
  const notes = motif.notes.slice();
  const playable: number[] = [];
  for (let i = 0; i < notes.length; i++) {
    if (notes[i] !== null) playable.push(i);
  }
  if (playable.length === 0) return motif;
  const wanted = rng() < 0.6 ? 2 : 1;
  const used = new Set<number>();
  let changed = 0;
  let guard = 0;
  while (changed < Math.min(wanted, playable.length) && guard < 64) {
    guard++;
    const i = playable[Math.floor(rng() * playable.length)]!;
    if (used.has(i)) continue;
    const v = notes[i];
    if (v === null) continue;
    const mv = rng() < 0.5 ? -1 : 1;
    const nv = Math.max(MELODY_LO, Math.min(MELODY_HI, v + mv));
    if (nv !== v) {
      notes[i] = nv;
      used.add(i);
      changed++;
    }
  }
  return { notes };
}

// Which layers sound per mood, and the tempo the transport glides to.
export interface LayerFlags {
  pad: boolean;
  bass: boolean;
  melody: boolean;
  perc: boolean;
}

export const MOOD_LAYERS: Record<Mood, LayerFlags> = {
  calm: { pad: true, bass: false, melody: true, perc: false },
  tension: { pad: true, bass: true, melody: true, perc: false },
  combat: { pad: true, bass: true, melody: false, perc: true },
};

export const MOOD_TEMPO: Record<Mood, number> = { calm: 68, tension: 90, combat: 124 };

// Biome changes timbre only, never harmony: filter cutoffs, oscillator
// types on the mono voices, melody echo amount and percussive level.
export interface BiomePreset {
  padFilter: number;
  melodyOsc: 'sine' | 'square' | 'sawtooth' | 'triangle';
  bassOsc: 'sine' | 'square' | 'sawtooth' | 'triangle';
  delayWet: number;
  percLevel: number;
}

export const BIOME_PRESETS: Record<string, BiomePreset> = {
  temperate: { padFilter: 1800, melodyOsc: 'triangle', bassOsc: 'sine', delayWet: 0.2, percLevel: 1 },
  taiga: { padFilter: 900, melodyOsc: 'sine', bassOsc: 'sine', delayWet: 0.3, percLevel: 0.8 },
  savanna: { padFilter: 3000, melodyOsc: 'square', bassOsc: 'triangle', delayWet: 0.35, percLevel: 1.1 },
  coast: { padFilter: 1200, melodyOsc: 'sine', bassOsc: 'sine', delayWet: 0.4, percLevel: 0.9 },
  prairie: { padFilter: 2400, melodyOsc: 'triangle', bassOsc: 'sine', delayWet: 0.25, percLevel: 1.15 },
};

export function biomePreset(key: string): BiomePreset {
  return BIOME_PRESETS[key] ?? BIOME_PRESETS['temperate']!;
}
