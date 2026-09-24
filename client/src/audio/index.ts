// Audio facade for the game client: one music bus and one effects bus,
// separate persistent volumes, everything synthesized (no samples).
// start() must be reached from a user gesture (browser autoplay policy).

import * as Tone from 'tone';

import { ToneMusic } from './music';
import { Sfx } from './sfx';
import { clampPct, loadVolumes, type Volumes } from './util';
import type { Mood } from './composition';

export type { Mood } from './composition';
export type { MusicEngine } from './music';

export interface AudioApi {
  start(): Promise<void>;
  stop(): void;
  setMood(mood: Mood): void;
  setBiome(biomeKey: string): void;
  setMusicVolume(pct: number): void;
  setSfxVolume(pct: number): void;
  click(): void;
  order(): void;
  attackOrder(): void;
  hit(): void;
  death(): void;
  place(): void;
  buildDone(): void;
  eraUp(): void;
  endMatch(win: boolean): void;
}

export function createAudio(seed: number, volumes: Volumes = loadVolumes()): AudioApi {
  const music = new ToneMusic(seed, clampPct(volumes.music));
  const sfx = new Sfx(clampPct(volumes.sfx));
  let started = false;

  return {
    async start(): Promise<void> {
      await Tone.start();
      await music.start();
      started = true;
    },
    stop(): void {
      music.stop();
    },
    setMood(mood: Mood): void {
      music.setMood(mood);
    },
    setBiome(biomeKey: string): void {
      music.setBiome(biomeKey);
    },
    setMusicVolume(pct: number): void {
      music.setVolume(clampPct(pct));
    },
    setSfxVolume(pct: number): void {
      sfx.setVolume(clampPct(pct));
    },
    click(): void {
      if (!started) return;
      sfx.click();
    },
    order(): void {
      if (!started) return;
      sfx.order();
    },
    attackOrder(): void {
      if (!started) return;
      sfx.attackOrder();
    },
    hit(): void {
      if (!started) return;
      sfx.hit();
    },
    death(): void {
      if (!started) return;
      sfx.death();
    },
    place(): void {
      if (!started) return;
      sfx.place();
    },
    buildDone(): void {
      if (!started) return;
      sfx.buildDone();
    },
    eraUp(): void {
      if (!started) return;
      sfx.eraUp();
    },
    endMatch(win: boolean): void {
      music.endMatch(win);
    },
  };
}
