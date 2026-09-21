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

  player0: 0xd94f4f,
  player1: 0x4f7fd9,
  player2: 0xd9a54f,
  player3: 0x9a4fd9,
  player4: 0x4fd9a5,
} as const;

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
