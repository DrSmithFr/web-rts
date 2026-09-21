import * as THREE from 'three';

// RTS camera: orbits a ground target with a fixed pitch, zoom, keyboard
// pan relative to the yaw, and clamping to the map.

const PITCH = 0.92; // radians above horizon
const MIN_DIST = 14;
const MAX_DIST_FACTOR = 0.55;

export class RtsCamera {
  yaw = 0;
  dist: number;
  target: THREE.Vector3;

  constructor(
    private readonly camera: THREE.PerspectiveCamera,
    private readonly span: number,
    private readonly groundHeight: (x: number, z: number) => number = () => 0,
  ) {
    this.dist = span * 0.1;
    this.target = new THREE.Vector3(span / 2, 0, span / 2);
  }

  pan(dxDx: number, dzWorld: number): void {
    const forward = new THREE.Vector3(-Math.sin(this.yaw), 0, -Math.cos(this.yaw));
    const right = new THREE.Vector3(-forward.z, 0, forward.x);
    this.target.addScaledVector(forward, dzWorld);
    this.target.addScaledVector(right, dxDx);
    const m = 4;
    this.target.x = THREE.MathUtils.clamp(this.target.x, -m, this.span + m);
    this.target.z = THREE.MathUtils.clamp(this.target.z, -m, this.span + m);
  }

  rotate(dyaw: number): void {
    this.yaw += dyaw;
  }

  // focus moves the orbit target to a ground position, staying clamped.
  focus(x: number, z: number): void {
    this.target.set(x, 0, z);
    this.pan(0, 0);
  }

  zoom(factor: number): void {
    this.dist = THREE.MathUtils.clamp(this.dist * factor, MIN_DIST, this.span * MAX_DIST_FACTOR);
  }

  update(): void {
    const h = this.groundHeight(this.target.x, this.target.z);
    const offset = new THREE.Vector3(
      Math.sin(this.yaw) * Math.cos(PITCH),
      Math.sin(PITCH),
      Math.cos(this.yaw) * Math.cos(PITCH),
    ).multiplyScalar(this.dist);
    this.camera.position.set(this.target.x + offset.x, h + offset.y, this.target.z + offset.z);
    this.camera.lookAt(this.target.x, h, this.target.z);
  }
}
