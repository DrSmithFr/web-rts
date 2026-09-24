// Main menu and game-setup screens. The menu runs before the sim boots
// and resolves with the URLSearchParams that should drive startup, so
// every boot path stays parameter-driven (and E2E tests can bypass the
// menu entirely via query string).

import { t } from '../i18n';
import { loadVolumes, saveVolumes } from '../audio/util';

const DIFFICULTIES = ['easy', 'standard', 'hard'];

export interface MenuResult {
  params: URLSearchParams;
}

function el(tag: string, cls: string, text?: string): HTMLElement {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

function field(label: string, input: HTMLElement): HTMLElement {
  const row = el('div', 'mrow');
  row.appendChild(el('label', '', label));
  row.appendChild(input);
  return row;
}

function select(id: string, options: string[], selected: string): HTMLSelectElement {
  const s = document.createElement('select');
  s.id = id;
  for (const o of options) {
    const opt = document.createElement('option');
    opt.value = o;
    opt.textContent = o;
    if (o === selected) opt.selected = true;
    s.appendChild(opt);
  }
  return s;
}

function textInput(id: string, value: string, placeholder = ''): HTMLInputElement {
  const i = document.createElement('input');
  i.id = id;
  i.value = value;
  i.placeholder = placeholder;
  return i;
}

export function menuFlow(defaultWs: string): Promise<MenuResult> {
  return new Promise((resolve) => {
    const root = document.getElementById('menu');
    if (!root) {
      resolve({ params: new URLSearchParams() });
      return;
    }
    root.hidden = false;
    const finish = (params: URLSearchParams) => {
      root.hidden = true;
      resolve({ params });
    };

    const show = (screen: HTMLElement) => {
      for (const c of Array.from(root.children)) (c as HTMLElement).style.display = 'none';
      screen.style.display = 'block';
    };

    const title = el('div', 'mtitle', 'AGE OF SEEDS');

    // main menu
    const main = el('div', 'mscreen');
    main.appendChild(title);
    const btnSolo = el('button', 'mbtn', 'Solo game');
    const btnMulti = el('button', 'mbtn', 'Multiplayer');
    const btnOptions = el('button', 'mbtn', 'Options');
    const btnCredits = el('button', 'mbtn', 'Credits');
    main.append(btnSolo, btnMulti, btnOptions, btnCredits);

    // solo configuration
    const solo = el('div', 'mscreen');
    solo.appendChild(el('div', 'mtitle', 'Solo game'));
    const seedIn = textInput('m-seed', '', 'blank = random');
    const botsSel = select('m-bots', ['0', '1', '2', '3'], '3');
    const diffSel = select('m-diff', DIFFICULTIES, 'standard');
    const mapSel = select('m-map', ['standard'], 'standard');
    mapSel.disabled = true;
    const victorySel = document.createElement('select');
    victorySel.disabled = true;
    for (const v of ['any (all enabled)']) {
      const o = document.createElement('option');
      o.value = v;
      o.textContent = v;
      victorySel.appendChild(o);
    }
    solo.appendChild(field('Map', mapSel));
    solo.appendChild(field('Seed', seedIn));
    solo.appendChild(field('Bots', botsSel));
    solo.appendChild(field('Bot difficulty', diffSel));
    solo.appendChild(field('Victory', victorySel));
    const soloStart = el('button', 'mbtn', 'Start');
    const soloBack = el('button', 'mbtn ghost', 'Back');
    solo.append(soloStart, soloBack);

    // multiplayer
    const multi = el('div', 'mscreen');
    multi.appendChild(el('div', 'mtitle', 'Multiplayer'));
    const wsIn = textInput('m-ws', defaultWs, 'ws://localhost:8787');
    const nameIn = textInput('m-name', localStorage.getItem('rts.name') || 'Player');
    multi.appendChild(field('Server', wsIn));
    multi.appendChild(field('Name', nameIn));
    const hostBots = select('m-hbots', ['0', '1', '2', '3'], '2');
    const hostDiff = select('m-hdiff', DIFFICULTIES, 'standard');
    multi.appendChild(field('Bots (host)', hostBots));
    multi.appendChild(field('Bot difficulty', hostDiff));
    const codeIn = textInput('m-code', sessionStorage.getItem('rts.code') || '', 'invite code');
    multi.appendChild(field('Join code', codeIn));
    const btnCreate = el('button', 'mbtn', 'Create room');
    const btnJoin = el('button', 'mbtn', 'Join room');
    const multiBack = el('button', 'mbtn ghost', 'Back');
    multi.append(btnCreate, btnJoin, multiBack);

    // options + credits
    const info = (heading: string, lines: string[]): HTMLElement => {
      const d = el('div', 'mscreen');
      d.appendChild(el('div', 'mtitle', heading));
      for (const l of lines) d.appendChild(el('div', 'mline', l));
      const back = el('button', 'mbtn ghost', 'Back');
      back.addEventListener('click', () => show(main));
      d.appendChild(back);
      return d;
    };
    const options = el('div', 'mscreen');
    options.appendChild(el('div', 'mtitle', 'Options'));
    const vols = loadVolumes();
    const volumeRow = (label: string, key: 'music' | 'sfx'): void => {
      const row = el('div', 'mrow');
      const lab = el('label', '', `${label}: ${vols[key]}`);
      const input = document.createElement('input');
      input.type = 'range';
      input.min = '0';
      input.max = '100';
      input.value = String(vols[key]);
      input.addEventListener('input', () => {
        const v = Number(input.value);
        vols[key] = v;
        saveVolumes(vols);
        lab.textContent = `${label}: ${v}`;
      });
      row.append(lab, input);
      options.appendChild(row);
    };
    volumeRow(t('menu.musicVolume'), 'music');
    volumeRow(t('menu.sfxVolume'), 'sfx');
    const optionsBack = el('button', 'mbtn ghost', 'Back');
    optionsBack.addEventListener('click', () => show(main));
    options.appendChild(optionsBack);
    const credits = info('Credits', [
      'Age of Seeds — a deterministic RTS study.',
      'Sim: Go/WASM. Client: TypeScript/Three.js.',
    ]);

    btnSolo.addEventListener('click', () => show(solo));
    btnMulti.addEventListener('click', () => show(multi));
    btnOptions.addEventListener('click', () => show(options));
    btnCredits.addEventListener('click', () => show(credits));
    soloBack.addEventListener('click', () => show(main));
    multiBack.addEventListener('click', () => show(main));

    soloStart.addEventListener('click', () => {
      const p = new URLSearchParams();
      const seed = (seedIn.value || '').trim();
      if (seed) p.set('seed', seed);
      p.set('bots', botsSel.value);
      p.set('difficulty', diffSel.value);
      p.set('menu', 'off');
      finish(p);
    });

    const startMulti = (mode: 'create' | 'join') => {
      const p = new URLSearchParams();
      p.set('ws', wsIn.value.trim());
      p.set('name', nameIn.value.trim() || 'Player');
      localStorage.setItem('rts.name', nameIn.value.trim() || 'Player');
      if (mode === 'create') {
        p.set('create', '1');
        p.set('bots', hostBots.value);
        p.set('difficulty', hostDiff.value);
      } else {
        p.set('code', codeIn.value.trim());
      }
      finish(p);
    };
    btnCreate.addEventListener('click', () => startMulti('create'));
    btnJoin.addEventListener('click', () => startMulti('join'));

    root.append(main, solo, multi, options, credits);
    show(main);
  });
}
