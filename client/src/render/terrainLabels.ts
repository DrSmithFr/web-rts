import { t, type MessageKey } from '../i18n';

// Terrain names keyed by the sim's tile type byte.
const KEYS: MessageKey[] = [
  'terrain.grass',
  'terrain.forest',
  'terrain.rock',
  'terrain.sand',
  'terrain.shallow',
  'terrain.deep',
];

export function terrainName(tile: number): string {
  const key = KEYS[tile];
  return key === undefined ? '???' : t(key);
}
