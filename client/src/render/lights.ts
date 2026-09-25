import * as THREE from 'three';

// FireLights gives every standing campfire a small emissive flame and a
// shared pool of point lights for the closest ones (constant light count,
// unused lights keep intensity 0 so shaders never recompile).

const MAX_LIGHTS = 8;

export interface FireSpot {
  id: number;
  x: number;
  y: number;
  z: number;
}

export class FireLights {
  private readonly flames = new Map<number, THREE.Group>();
  private readonly lights: THREE.PointLight[] = [];
  private readonly flameGeo = new THREE.ConeGeometry(0.12, 0.3, 6);
  private readonly coreGeo = new THREE.ConeGeometry(0.06, 0.2, 6);
  private readonly flameMat = new THREE.MeshBasicMaterial({
    color: 0xff9a38,
    transparent: true,
    opacity: 0.9,
  });
  private readonly coreMat = new THREE.MeshBasicMaterial({ color: 0xffe9a0 });

  constructor(private readonly scene: THREE.Scene) {
    this.flameGeo.translate(0, 0.18, 0);
    this.coreGeo.translate(0, 0.14, 0);
    for (let i = 0; i < MAX_LIGHTS; i++) {
      const l = new THREE.PointLight(0xffa850, 0, 7, 1.7);
      this.scene.add(l);
      this.lights.push(l);
    }
  }

  // night is 0 at noon and 1 at midnight; t is wall-clock seconds.
  update(campfires: FireSpot[], camX: number, camZ: number, night: number, t: number): void {
    const seen = new Set<number>();
    for (const c of campfires) {
      seen.add(c.id);
      let g = this.flames.get(c.id);
      if (!g) {
        g = new THREE.Group();
        const f = new THREE.Mesh(this.flameGeo, this.flameMat);
        const core = new THREE.Mesh(this.coreGeo, this.coreMat);
        g.add(f, core);
        g.position.set(c.x, c.y, c.z);
        this.scene.add(g);
        this.flames.set(c.id, g);
      }
      const seed = (c.id * 13.7) % 6.28;
      const fl = 0.85 + 0.25 * Math.sin(t * 11 + seed) * Math.sin(t * 5.3 + seed * 0.5);
      g.scale.set(1 + 0.12 * (fl - 1), fl, 1 + 0.12 * (fl - 1));
      g.rotation.y = t * 2 + seed;
    }
    for (const [id, g] of this.flames) {
      if (!seen.has(id)) {
        this.scene.remove(g);
        this.flames.delete(id);
      }
    }

    const near = campfires
      .slice()
      .sort((a, b) => (a.x - camX) ** 2 + (a.z - camZ) ** 2 - ((b.x - camX) ** 2 + (b.z - camZ) ** 2))
      .slice(0, MAX_LIGHTS);
    for (let i = 0; i < this.lights.length; i++) {
      const l = this.lights[i];
      const c = near[i];
      if (!c) {
        l.intensity = 0;
        continue;
      }
      l.position.set(c.x, c.y + 0.5, c.z);
      const seed = (c.id * 7.3) % 6.28;
      const flicker = 0.8 + 0.2 * Math.sin(t * 13 + seed) * Math.sin(t * 6.1 + seed);
      l.intensity = (0.5 + 2.5 * night) * flicker;
    }
  }
}
