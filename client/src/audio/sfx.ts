// Procedural sound effects: short one-shots from reused synths, on a bus
// separate from the music. Bursty combat events are rate-limited.

import * as Tone from 'tone';

import { Throttle, clampPct } from './util';

export class Sfx {
  private out?: Tone.Gain;
  private blip?: Tone.Synth;
  private thud?: Tone.MembraneSynth;
  private tap?: Tone.NoiseSynth;
  private deathNoise?: Tone.NoiseSynth;
  private vol: number;
  private thHit = new Throttle(90);
  private thDeath = new Throttle(150);

  constructor(volumePct: number) {
    this.vol = clampPct(volumePct) / 100;
  }

  setVolume(volumePct: number): void {
    this.vol = clampPct(volumePct) / 100;
    if (this.out) this.out.gain.rampTo(this.vol * 0.9, 0.2);
  }

  private ensure(): boolean {
    if (this.out) return true;
    const out = new Tone.Gain(this.vol * 0.9).toDestination();
    this.blip = new Tone.Synth({
      oscillator: { type: 'square' },
      envelope: { attack: 0.002, decay: 0.06, sustain: 0, release: 0.05 },
    }).connect(out);
    this.thud = new Tone.MembraneSynth({
      octaves: 4,
      envelope: { attack: 0.001, decay: 0.25, sustain: 0.15, release: 0.3 },
    }).connect(out);
    this.tap = new Tone.NoiseSynth({
      noise: { type: 'white' },
      envelope: { attack: 0.001, decay: 0.05, sustain: 0, release: 0.02 },
    }).connect(out);
    this.deathNoise = new Tone.NoiseSynth({
      noise: { type: 'pink' },
      envelope: { attack: 0.005, decay: 0.18, sustain: 0, release: 0.1 },
    }).connect(out);
    this.out = out;
    return true;
  }

  private tone(note: string, delay = 0, dur: string = '16n', vel = 0.4): void {
    const when = delay > 0 ? Tone.now() + delay : Tone.now();
    this.blip?.triggerAttackRelease(note, dur, when, vel);
  }

  click(): void {
    if (!this.ensure()) return;
    this.tone('E5', 0, '32n', 0.2);
  }

  order(): void {
    if (!this.ensure()) return;
    this.tone('A4', 0, '32n', 0.25);
    this.tone('D5', 0.07, '32n', 0.25);
  }

  attackOrder(): void {
    if (!this.ensure()) return;
    this.tone('D5', 0, '32n', 0.3);
    this.tone('G4', 0.08, '32n', 0.3);
  }

  hit(): void {
    if (!this.thHit.allow() || !this.ensure()) return;
    this.tap?.triggerAttackRelease('32n', Tone.now(), 0.22);
  }

  death(): void {
    if (!this.thDeath.allow() || !this.ensure()) return;
    this.deathNoise?.triggerAttackRelease('8n', Tone.now(), 0.35);
  }

  place(): void {
    if (!this.ensure()) return;
    this.thud?.triggerAttackRelease('C2', '8n', Tone.now(), 0.7);
    this.tap?.triggerAttackRelease('32n', Tone.now() + 0.02, 0.18);
  }

  buildDone(): void {
    if (!this.ensure()) return;
    this.tone('G5', 0, '16n', 0.3);
    this.tone('C6', 0.09, '16n', 0.3);
  }

  eraUp(): void {
    if (!this.ensure()) return;
    this.tone('C5', 0, '16n', 0.3);
    this.tone('E5', 0.1, '16n', 0.3);
    this.tone('G5', 0.2, '16n', 0.3);
    this.tone('C6', 0.32, '8n', 0.35);
  }
}
