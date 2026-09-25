import { describe, expect, it } from 'vitest';

import {
  BIOME_PRESETS,
  MELODY_HI,
  MELODY_LO,
  MINOR_STEPS,
  MOOD_LAYERS,
  MOOD_TEMPO,
  MOTIF_LEN,
  biomePreset,
  makeMotif,
  mulberry32,
  mutateMotif,
  nextBlock,
  nextDegree,
  rootFromSeed,
  scaleMidi,
} from './composition';

describe('mulberry32', () => {
  it('reproduces the same sequence from the same seed', () => {
    const a = mulberry32(42);
    const b = mulberry32(42);
    const va = [a(), a(), a(), a(), a()];
    const vb = [b(), b(), b(), b(), b()];
    expect(va).toEqual(vb);
  });

  it('separates different seeds', () => {
    expect(mulberry32(1)()).not.toBe(mulberry32(2)());
  });
});

describe('scale', () => {
  it('maps degrees onto fixed-scale pitches, octaves included', () => {
    expect(scaleMidi(45, 0)).toBe(45);
    expect(scaleMidi(45, 7)).toBe(57);
    expect(scaleMidi(45, 8)).toBe(59);
    expect(scaleMidi(45, -1)).toBe(45 + MINOR_STEPS[6] - 12);
  });

  it('picks a deterministic tonic per seed', () => {
    expect(rootFromSeed(7)).toBe(rootFromSeed(7));
    expect(rootFromSeed(7)).toBeGreaterThanOrEqual(45);
    expect(rootFromSeed(7)).toBeLessThan(57);
  });
});

describe('progression', () => {
  it('builds blocks of 4 or 8 diatonic chords with no immediate repeat', () => {
    for (let seed = 0; seed < 40; seed++) {
      const rng = mulberry32(seed);
      let prev = 0;
      for (let b = 0; b < 8; b++) {
        const block = nextBlock(rng, prev);
        expect([4, 8]).toContain(block.length);
        let before = prev;
        for (const d of block) {
          expect(d).toBeGreaterThanOrEqual(0);
          expect(d).toBeLessThan(7);
          expect(d).not.toBe(before);
          before = d;
        }
        prev = block[block.length - 1]!;
      }
    }
  });

  it('walks by musical offsets only', () => {
    const rng = mulberry32(9);
    for (let i = 0; i < 100; i++) {
      const prev = Math.floor(rng() * 7);
      const d = nextDegree(rng, prev);
      expect(d).not.toBe(prev);
    }
  });
});

describe('melody motif', () => {
  it('starts on a note inside the melody register, with bounded motion', () => {
    let totalGaps = 0;
    let totalSmall = 0;
    for (let seed = 0; seed < 60; seed++) {
      const { notes } = makeMotif(mulberry32(seed));
      expect(notes.length).toBe(MOTIF_LEN);
      const first = notes[0];
      expect(first).not.toBeNull();
      expect(first!).toBeGreaterThanOrEqual(MELODY_LO);
      expect(first!).toBeLessThanOrEqual(MELODY_HI);

      let run = 0;
      let last: number | null = null;
      for (const n of notes) {
        if (n === null) {
          run++;
          expect(run).toBeLessThanOrEqual(2);
          continue;
        }
        run = 0;
        expect(n).toBeGreaterThanOrEqual(MELODY_LO);
        expect(n).toBeLessThanOrEqual(MELODY_HI);
        if (last !== null) {
          const gap = Math.abs(n - last);
          expect(gap).toBeLessThanOrEqual(4);
          totalGaps++;
          if (gap <= 2) totalSmall++;
        }
        last = n;
      }
      expect(notes.filter((n) => n !== null).length).toBeGreaterThanOrEqual(8);
    }
    // motion is mostly by small intervals, across all seeds
    expect(totalSmall).toBeGreaterThan(totalGaps * 0.6);
  });

  it('repeats mutate small: one or two notes change, structure stays', () => {
    for (let seed = 0; seed < 40; seed++) {
      const rng = mulberry32(seed * 7 + 1);
      let m = makeMotif(rng);
      for (let r = 0; r < 6; r++) {
        const before = m.notes;
        m = mutateMotif(rng, m);
        expect(m.notes.length).toBe(MOTIF_LEN);
        let changed = 0;
        for (let i = 0; i < MOTIF_LEN; i++) {
          const a = before[i];
          const b = m.notes[i];
          if (a === null || b === null) {
            expect(b).toBe(a); // rests never appear or vanish
            continue;
          }
          if (a !== b) {
            changed++;
            expect(b).toBeGreaterThanOrEqual(MELODY_LO);
            expect(b).toBeLessThanOrEqual(MELODY_HI);
            expect(Math.abs(b - a)).toBeLessThanOrEqual(2);
          }
        }
        expect(changed).toBeGreaterThanOrEqual(1);
        expect(changed).toBeLessThanOrEqual(2);
      }
    }
  });
});

describe('mood and biome tables', () => {
  it('activates layers exactly as the design table', () => {
    expect(MOOD_LAYERS.calm).toEqual({ pad: true, bass: false, melody: true, perc: false });
    expect(MOOD_LAYERS.tension).toEqual({ pad: true, bass: true, melody: true, perc: false });
    expect(MOOD_LAYERS.combat).toEqual({ pad: true, bass: true, melody: false, perc: true });
  });

  it('tempo rises with intensity', () => {
    expect(MOOD_TEMPO.calm).toBeLessThan(MOOD_TEMPO.tension);
    expect(MOOD_TEMPO.tension).toBeLessThan(MOOD_TEMPO.combat);
  });

  it('every listed biome has a timbre preset, unknown ones fall back', () => {
    for (const key of ['temperate', 'taiga', 'savanna', 'coast', 'prairie']) {
      expect(biomePreset(key)).toBe(BIOME_PRESETS[key]);
    }
    expect(biomePreset('volcano')).toBe(BIOME_PRESETS['temperate']);
  });
});
