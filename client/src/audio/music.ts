// Tone.js music engine: one transport, four synth layers (pad, bass,
// melody, percussion) scheduled on an 8th-note grid over a seeded chord
// progression. Mood swaps layers at the next bar with gain fades; biome
// only re-tunes the synths. Match end plays a victory or defeat theme.

import * as Tone from 'tone';

import {
  biomePreset,
  makeMotif,
  mutateMotif,
  MOOD_LAYERS,
  MOOD_TEMPO,
  mulberry32,
  nextBlock,
  rootFromSeed,
  scaleMidi,
  type LayerFlags,
  type Motif,
  type Mood,
  type Rng,
} from './composition';

export type { Mood } from './composition';

export interface MusicEngine {
  start(): Promise<void>;
  stop(): void;
  setMood(mood: Mood): void;
  setVolume(volume: number): void; // 0..100, music-only bus
  setBiome(biome: string): void;
  endMatch(win: boolean): void;
}

interface ThemeNote {
  s: number; // 8th-note step within the theme
  k: 'mel' | 'bass' | 'chord';
  n: number; // semitones from the tonic
  d: string; // tone.js duration
}

const THEME_STEPS = 24;

// Victory: rising major fanfare over tonic then dominant.
const VICTORY: ThemeNote[] = [
  { s: 0, k: 'mel', n: 0, d: '8n' },
  { s: 2, k: 'mel', n: 4, d: '8n' },
  { s: 4, k: 'mel', n: 7, d: '8n' },
  { s: 6, k: 'mel', n: 12, d: '4n' },
  { s: 0, k: 'bass', n: 0, d: '1m' },
  { s: 12, k: 'mel', n: 7, d: '8n' },
  { s: 14, k: 'mel', n: 11, d: '8n' },
  { s: 16, k: 'mel', n: 12, d: '4n' },
  { s: 18, k: 'mel', n: 16, d: '8n' },
  { s: 20, k: 'mel', n: 19, d: '4n' },
  { s: 16, k: 'bass', n: 7, d: '1m' },
  { s: 16, k: 'chord', n: 0, d: '2m' },
  { s: 16, k: 'chord', n: 4, d: '2m' },
  { s: 16, k: 'chord', n: 7, d: '2m' },
];

// Defeat: slow minor descent onto a low tonic.
const DEFEAT: ThemeNote[] = [
  { s: 0, k: 'mel', n: 12, d: '8n' },
  { s: 3, k: 'mel', n: 10, d: '8n' },
  { s: 6, k: 'mel', n: 7, d: '4n' },
  { s: 10, k: 'mel', n: 5, d: '8n' },
  { s: 13, k: 'mel', n: 3, d: '8n' },
  { s: 16, k: 'mel', n: 0, d: '2n' },
  { s: 0, k: 'bass', n: 0, d: '2m' },
  { s: 16, k: 'chord', n: 0, d: '2m' },
  { s: 16, k: 'chord', n: 3, d: '2m' },
  { s: 16, k: 'chord', n: 7, d: '2m' },
];

// Base layer levels; also the hard cap on simultaneous voices
// (3 pad + 1 bass + 1 melody + 3 perc).
const BASE = { pad: 0.26, bass: 0.38, melody: 0.4, perc: 0.45 };
const FADE = 1.8;

export class ToneMusic implements MusicEngine {
  private readonly rng: Rng;
  private readonly root: number;
  private mood: Mood = 'calm';
  private pendingMood: Mood | null = null;
  private volume: number;
  private percLevel = 1;
  private running = false;
  private over = false;

  private block: number[];
  private blockPos = 0;
  private degree: number;
  private motif: Motif;
  private motifAge = 99;
  private step = 0;

  private out?: Tone.Gain;
  private pad?: Tone.PolySynth;
  private padFilter?: Tone.Filter;
  private padGain?: Tone.Gain;
  private bass?: Tone.MonoSynth;
  private bassGain?: Tone.Gain;
  private melody?: Tone.MonoSynth;
  private melDelay?: Tone.FeedbackDelay;
  private melodyGain?: Tone.Gain;
  private kick?: Tone.MembraneSynth;
  private hat?: Tone.NoiseSynth;
  private snare?: Tone.NoiseSynth;
  private percGain?: Tone.Gain;
  private seq?: Tone.Sequence;
  private themeSeq?: Tone.Sequence;
  private on: LayerFlags = { pad: false, bass: false, melody: false, perc: false };

  constructor(seed: number, volumePct: number) {
    this.rng = mulberry32((seed ^ 0x5eed5eed) >>> 0);
    this.root = rootFromSeed(seed);
    this.volume = Math.max(0, Math.min(100, volumePct)) / 100;
    this.block = nextBlock(this.rng, 0);
    this.degree = this.block[0]!;
    this.motif = makeMotif(this.rng);
  }

  async start(): Promise<void> {
    await Tone.start();
    if (this.running) return;
    if (!this.out) this.initVoices();
    this.running = true;
    this.over = false;
    this.on = { pad: false, bass: false, melody: false, perc: false };
    Tone.Transport.bpm.value = MOOD_TEMPO[this.mood];
    this.seq = new Tone.Sequence(
      (time: number, i: number) => this.tick(time, i),
      [0, 1, 2, 3, 4, 5, 6, 7],
      '8n',
    );
    this.seq.start(0);
    this.applyMood();
    await Tone.Transport.start('+0.1');
  }

  stop(): void {
    this.running = false;
    this.seq?.stop();
    this.themeSeq?.stop();
    this.out?.gain.rampTo(0, 0.4);
    Tone.Transport.stop();
  }

  setMood(mood: Mood): void {
    if (mood === this.mood && this.pendingMood === null) return;
    if (!this.running) {
      this.mood = mood;
      this.pendingMood = null;
      return;
    }
    this.pendingMood = mood;
  }

  setVolume(volume: number): void {
    this.volume = Math.max(0, Math.min(100, volume)) / 100;
    this.out?.gain.rampTo(this.volume, 0.3);
  }

  // Biome re-tunes the voices; harmony is untouched.
  setBiome(biome: string): void {
    if (!this.out) return;
    const p = biomePreset(biome);
    this.padFilter?.frequency.rampTo(p.padFilter, 1.5);
    this.melDelay?.wet.rampTo(p.delayWet, 1.5);
    this.melody?.set({ oscillator: { type: p.melodyOsc } });
    this.bass?.set({ oscillator: { type: p.bassOsc } });
    this.percLevel = p.percLevel;
    if (this.on.perc) this.percGain?.gain.rampTo(BASE.perc * this.percLevel, 1.5);
  }

  endMatch(win: boolean): void {
    if (this.over) return;
    this.over = true;
    this.seq?.stop();
    const notes = win ? VICTORY : DEFEAT;
    this.percGain?.gain.rampTo(0, 0.3);
    this.padGain?.gain.rampTo(0.2, 0.5);
    this.bassGain?.gain.rampTo(0.32, 0.5);
    this.melodyGain?.gain.rampTo(0.5, 0.3);
    Tone.Transport.bpm.rampTo(96, 2);
    const steps: number[] = [];
    for (let k = 0; k < THEME_STEPS; k++) steps.push(k);
    this.themeSeq = new Tone.Sequence(
      (time: number, i: number) => this.themeTick(time, i, notes),
      steps,
      '8n',
    );
    this.themeSeq.start(0);
  }

  // --- internals ---------------------------------------------------------

  private initVoices(): void {
    this.out = new Tone.Gain(this.volume).toDestination();

    this.padGain = new Tone.Gain(0).connect(this.out);
    this.padFilter = new Tone.Filter({ type: 'lowpass', frequency: 1800 }).connect(this.padGain);
    this.pad = new Tone.PolySynth({
      maxPolyphony: 3,
      voice: Tone.Synth,
      options: {
        oscillator: { type: 'triangle' },
        envelope: { attack: 1.2, decay: 0.5, sustain: 0.85, release: 2.5 },
      },
    }).connect(this.padFilter);

    this.bassGain = new Tone.Gain(0).connect(this.out);
    this.bass = new Tone.MonoSynth({
      oscillator: { type: 'sine' },
      filterEnvelope: { baseFrequency: 90, octaves: 1.6, release: 0.2 },
      envelope: { attack: 0.02, decay: 0.3, sustain: 0.6, release: 0.4 },
    }).connect(this.bassGain);

    this.melodyGain = new Tone.Gain(0).connect(this.out);
    this.melDelay = new Tone.FeedbackDelay(0.24, 0.35).connect(this.melodyGain);
    this.melody = new Tone.MonoSynth({
      oscillator: { type: 'triangle' },
      filterEnvelope: { baseFrequency: 500, octaves: 2, release: 0.3 },
      envelope: { attack: 0.01, decay: 0.25, sustain: 0.35, release: 0.5 },
    }).connect(this.melDelay);

    this.percGain = new Tone.Gain(0).connect(this.out);
    this.kick = new Tone.MembraneSynth({
      octaves: 5,
      envelope: { attack: 0.002, decay: 0.28, sustain: 0.12, release: 0.3 },
    }).connect(this.percGain);
    this.hat = new Tone.NoiseSynth({
      noise: { type: 'white' },
      envelope: { attack: 0.001, decay: 0.04, sustain: 0, release: 0.02 },
    }).connect(this.percGain);
    this.snare = new Tone.NoiseSynth({
      noise: { type: 'white' },
      envelope: { attack: 0.002, decay: 0.12, sustain: 0, release: 0.05 },
    }).connect(this.percGain);
  }

  // One 8th-note step of the groove; bar bookkeeping happens on step 0.
  private tick(time: number, i: number): void {
    if (this.over) return;
    if (i === 0) this.bar(time);
    if (this.on.melody) {
      const n = this.motif.notes[this.step % 16];
      if (n !== null && n !== undefined) {
        this.melody?.triggerAttackRelease(this.note(this.root + 24, n), '8n', time, 0.9);
      }
    }
    if (this.on.bass) {
      const hit = this.mood === 'combat' ? i % 2 === 0 : i === 0 || i === 6;
      if (hit) {
        this.bass?.triggerAttackRelease(this.note(this.root - 12, this.degree), '4n', time, 0.9);
      }
    }
    if (this.on.perc) {
      if (i % 2 === 1) this.hat?.triggerAttackRelease('16n', time, i % 4 === 1 ? 0.5 : 0.35);
      if (i === 0 || i === 4) this.kick?.triggerAttackRelease('A1', '8n', time, 0.9);
      if (i === 2 || i === 6) this.snare?.triggerAttackRelease('8n', time, 0.45);
    }
    this.step++;
  }

  private bar(time: number): void {
    if (this.pendingMood !== null) {
      this.mood = this.pendingMood;
      this.pendingMood = null;
      this.applyMood();
    }
    this.blockPos++;
    if (this.blockPos >= this.block.length) {
      this.block = nextBlock(this.rng, this.degree);
      this.blockPos = 0;
    }
    this.degree = this.block[this.blockPos]!;
    this.motifAge++;
    if (this.motifAge >= 12) {
      this.motif = makeMotif(this.rng);
      this.motifAge = 0;
    } else if (this.motifAge % 2 === 0) {
      this.motif = mutateMotif(this.rng, this.motif);
    }
    if (this.on.pad) {
      const chord = [0, 2, 4].map((o) => this.note(this.root + 12, this.degree + o));
      this.pad?.triggerAttackRelease(chord, '1m', time, 0.5);
    }
  }

  private applyMood(): void {
    const want = MOOD_LAYERS[this.mood];
    const nodes: Record<keyof LayerFlags, { node: Tone.Gain | undefined; base: number }> = {
      pad: { node: this.padGain, base: BASE.pad },
      bass: { node: this.bassGain, base: BASE.bass },
      melody: { node: this.melodyGain, base: BASE.melody },
      perc: { node: this.percGain, base: BASE.perc * this.percLevel },
    };
    for (const key of Object.keys(want) as (keyof LayerFlags)[]) {
      this.on[key] = want[key];
      const target = want[key] ? nodes[key].base : 0;
      nodes[key].node?.gain.rampTo(target, FADE);
    }
    Tone.Transport.bpm.rampTo(MOOD_TEMPO[this.mood], 3);
  }

  private themeTick(time: number, i: number, notes: ThemeNote[]): void {
    for (const e of notes) {
      if (e.s !== i) continue;
      if (e.k === 'mel') {
        this.melody?.triggerAttackRelease(this.hz(this.root + 24 + e.n), e.d, time, 0.9);
      } else if (e.k === 'bass') {
        this.bass?.triggerAttackRelease(this.hz(this.root - 12 + e.n), e.d, time, 0.8);
      } else {
        this.pad?.triggerAttackRelease(this.hz(this.root + 12 + e.n), e.d, time, 0.5);
      }
    }
    if (i === THEME_STEPS - 1) {
      this.themeSeq?.stop();
      this.out?.gain.rampTo(0, 3);
      Tone.Transport.stop(time);
    }
  }

  private note(baseMidi: number, degree: number): string {
    return this.hz(scaleMidi(baseMidi, degree));
  }

  private hz(midi: number): string {
    return Tone.Midi(midi).toNote();
  }
}
