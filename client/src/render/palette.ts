// The whole game shares one flat 16-color palette (art direction).
// Terrain gets six entries, object parts and markers five, the rest are
// reserved for the eight player colors (owner tinting lands in phase 5).

export const PALETTE = {
  grass: 0x6ab04c,
  forest: 0x4a8a3c,
  sand: 0xd8c37a,
  rock: 0x8d949e,
  shallow: 0x5b9bd5,
  deep: 0x2e5f9e,

  trunk: 0x6e4a2f,
  leaf: 0x2f7d2f,
  stone: 0x787f88,
  marker: 0xe8c14a,
  select: 0xffffff,

  gold: 0xf1c40f,
  berry: 0xc0396b,
  hut: 0xa9773f,
  campfire: 0xd35400,
  storehouse: 0x8e6b3f,

  player0: 0xd94f4f,
  player1: 0x4f7fd9,
  player2: 0xd9a54f,
  player3: 0x9a4fd9,
  player4: 0x4fd9a5,
  player5: 0xd94fb2,
  player6: 0x4fd9d9,
  player7: 0xb8d94f,

  deer: 0xb08a5a,
  boar: 0x6f6257,
  mount: 0x6b4f3a,
} as const;

// Scout mount color (kept as a named export for the actors module).
export const MOUNT_COLOR: number = PALETTE.mount;

// Player body colors, indexed by owner id.
export const PLAYER_COLORS: number[] = [
  PALETTE.player0,
  PALETTE.player1,
  PALETTE.player2,
  PALETTE.player3,
  PALETTE.player4,
  PALETTE.player5,
  PALETTE.player6,
  PALETTE.player7,
];

export const TERRAIN_COLORS: number[] = [
  PALETTE.grass,
  PALETTE.forest,
  PALETTE.rock,
  PALETTE.sand,
  PALETTE.shallow,
  PALETTE.deep,
];

// Terrain type byte values mirrored from sim/terrain.
export const TERRAIN = {
  Grass: 0,
  Forest: 1,
  Rock: 2,
  Sand: 3,
  Shallow: 4,
  Deep: 5,
} as const;

export const HEIGHT_SCALE = 0.35;
