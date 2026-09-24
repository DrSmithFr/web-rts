// The curated palette (art direction): flat colors only, one entry per
// material role. Terrain owns six entries, decor four (tuft, flower, reed,
// stone), units and weapons nine, buildings four, fire two, plus the eight
// player colors and the two selection markers. Nothing outside this file
// introduces a new hue.

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

  // decor scatter
  tuft: 0x589f44,
  flower: 0xe88f7f,
  reed: 0x77a05a,

  // unit materials
  skin: 0xc98f6a,
  steel: 0xb9bfc9,
  iron: 0x6b7280,
  leather: 0x6e4a2f,
  cloth: 0x4a4438,
  mane: 0x46372c,

  // building materials
  thatch: 0xb98a44,
  plank: 0x8a6a42,
  plaster: 0xcfc4ae,
  marble: 0xd9d4c8,

  // fire
  flame: 0xffa23a,
  ember: 0xff6a2a,

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

  // biome ground tints (blended into grass/forest tiles)
  biomeTemperate: 0x5f9e46,
  biomeTaiga: 0x3e6f55,
  biomeSavanna: 0x9c9a4e,
  biomeCoast: 0x5fae8a,
  biomePrairie: 0x9fc25c,

  // tree species
  spruceLeaf: 0x245c3a,
  birchLeaf: 0x7ab648,
  birchTrunk: 0xd8d2c2,
  palmLeaf: 0x3f9e5f,
  palmTrunk: 0x8f6a3f,
  baobabLeaf: 0x6f8f3a,
  baobabTrunk: 0x9c8260,
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

// Carried-resource colors, indexed by the sim resource kind byte
// (0 = nothing, 1 wood, 2 stone, 3 food, 4 gold, 5 oil).
export const RESOURCE_COLORS: number[] = [
  PALETTE.stone,
  PALETTE.trunk,
  PALETTE.stone,
  PALETTE.berry,
  PALETTE.gold,
  0x2c2c34,
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

// Biome ground tints, indexed by the sim biome id byte (sim/biome).
export const BIOME_COLORS: number[] = [
  PALETTE.biomeTemperate,
  PALETTE.biomeTaiga,
  PALETTE.biomeSavanna,
  PALETTE.biomeCoast,
  PALETTE.biomePrairie,
];

// Biome id bytes mirrored from sim/biome.
export const BIOME = {
  Temperate: 0,
  Taiga: 1,
  Savanna: 2,
  Coast: 3,
  Prairie: 4,
} as const;

// Tree species id bytes mirrored from sim/tree.
export const TREE = {
  None: 0,
  Spruce: 1,
  Birch: 2,
  Palm: 3,
  Baobab: 4,
} as const;

export const HEIGHT_SCALE = 0.35;
