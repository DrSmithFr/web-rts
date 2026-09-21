import { TERRAIN_COLORS } from './palette';
import { FogUnexplored, FogVisible, type FogView } from './fogView';

// Canvas minimap: baked terrain layer, per-frame fog + unit overlay and
// click-to-focus. 1 pixel per tile, scaled up without smoothing.

export interface MinimapEntity {
  x: number;
  y: number;
  owner: number;
}

export class Minimap {
  private readonly ctx: CanvasRenderingContext2D;
  private readonly terrain: HTMLCanvasElement;
  private readonly fogImg: ImageData;

  constructor(
    private readonly canvas: HTMLCanvasElement,
    private readonly size: number,
    tiles: Uint8Array,
  ) {
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('2d context unavailable');
    this.ctx = ctx;
    this.terrain = document.createElement('canvas');
    this.terrain.width = size;
    this.terrain.height = size;
    const tctx = this.terrain.getContext('2d')!;
    const img = tctx.createImageData(size, size);
    for (let i = 0; i < size * size; i++) {
      const c = TERRAIN_COLORS[tiles[i]] ?? 0xff00ff;
      img.data[i * 4] = (c >> 16) & 255;
      img.data[i * 4 + 1] = (c >> 8) & 255;
      img.data[i * 4 + 2] = c & 255;
      img.data[i * 4 + 3] = 255;
    }
    tctx.putImageData(img, 0, 0);
    this.fogImg = this.ctx.createImageData(size, size);
    canvas.addEventListener('pointerdown', (e) => this.onClick(e));
  }

  onPick: ((x: number, y: number) => void) | null = null;

  private onClick(e: PointerEvent): void {
    if (!this.onPick) return;
    const rect = this.canvas.getBoundingClientRect();
    const x = Math.floor(((e.clientX - rect.left) / rect.width) * this.size);
    const y = Math.floor(((e.clientY - rect.top) / rect.height) * this.size);
    if (x >= 0 && y >= 0 && x < this.size && y < this.size) this.onPick(x, y);
  }

  draw(
    fog: FogView | null,
    units: MinimapEntity[],
    camera: { x: number; z: number; view: number },
    relics: { x: number; y: number; owner: number }[] = [],
  ): void {
    const { ctx } = this;
    const s = this.size;
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    ctx.drawImage(this.terrain, 0, 0, this.canvas.width, this.canvas.height);

    if (fog) {
      const d = this.fogImg.data;
      for (let i = 0; i < s * s; i++) {
        const st = fog.at(i % s, Math.floor(i / s));
        const a = st === FogVisible ? 0 : st === FogUnexplored ? 250 : 130;
        d[i * 4] = 6;
        d[i * 4 + 1] = 8;
        d[i * 4 + 2] = 14;
        d[i * 4 + 3] = a;
      }
      // fog layer drawn through a full-size scratch canvas
      const scratch = this.scratch();
      const sctx = scratch.getContext('2d')!;
      sctx.putImageData(this.fogImg, 0, 0);
      ctx.drawImage(scratch, 0, 0, this.canvas.width, this.canvas.height);
    }

    const k = this.canvas.width / s;
    for (const u of units) {
      ctx.fillStyle = u.owner === 0 ? '#ffffff' : u.owner >= 8 ? '#9aa0a8' : '#d94f4f';
      ctx.fillRect(u.x * k - 1, u.y * k - 1, 2.5, 2.5);
    }
    for (const r of relics) {
      if (fog && fog.at(r.x, r.y) === FogUnexplored) continue;
      ctx.fillStyle = '#e8c34a';
      ctx.fillRect(r.x * k - 2, r.y * k - 2, 4, 4);
      if (r.owner !== 255) {
        ctx.fillStyle = r.owner === 0 ? '#ffffff' : '#d94f4f';
        ctx.fillRect(r.x * k - 1, r.y * k - 1, 2, 2);
      }
    }
    ctx.strokeStyle = 'rgba(255,255,255,0.85)';
    ctx.lineWidth = 1;
    ctx.strokeRect(
      (camera.x - camera.view / 2) * k,
      (camera.z - camera.view / 2) * k,
      camera.view * k,
      camera.view * k,
    );
  }

  private scratchCache: HTMLCanvasElement | null = null;

  private scratch(): HTMLCanvasElement {
    if (!this.scratchCache) {
      this.scratchCache = document.createElement('canvas');
      this.scratchCache.width = this.size;
      this.scratchCache.height = this.size;
    }
    return this.scratchCache;
  }
}
