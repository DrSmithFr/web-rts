import * as THREE from 'three';

// RTS camera: orbits a ground target with a fixed pitch, eased zoom and
// eased follow of the pan target, keyboard pan relative to the yaw, and
// clamping to the map.

const PITCH = 0.92; // radians above horizon
const MIN_DIST = 14;
// at max zoom the whole map fits the frame without exposing sky walls
const MAX_DIST_FACTOR = 0.42;
const FOLLOW = 0.16;
const ZOOM_GLIDE = 0.16;

export class RtsCamera {
  yaw = 0;
  dist: number;
  target: THREE.Vector3;

  private targetYaw: number;
  private targetDist: number;
  private glide: THREE.Vector3;

  constructor(
    private readonly camera: THREE.PerspectiveCamera,
    private readonly span: number,
    private readonly groundHeight: (x: number, z: number) => number = () => 0,
  ) {
    this.dist = span * 0.1;
    this.targetDist = this.dist;
    this.target = new THREE.Vector3(span / 2, 0, span / 2);
    this.glide = this.target.clone();
    this.targetYaw = 0;
  }

  pan(dxDx: number, dzWorld: number): void {
    const forward = new THREE.Vector3(-Math.sin(this.targetYaw), 0, -Math.cos(this.targetYaw));
    const right = new THREE.Vector3(-forward.z, 0, forward.x);
    this.target.addScaledVector(forward, dzWorld);
    this.target.addScaledVector(right, dxDx);
    const m = 4;
    this.target.x = THREE.MathUtils.clamp(this.target.x, -m, this.span + m);
    this.target.z = THREE.MathUtils.clamp(this.target.z, -m, this.span + m);
  }

  rotate(dyaw: number): void {
    this.targetYaw += dyaw;
  }

  // focus moves the orbit target to a ground position, staying clamped.
  focus(x: number, z: number): void {
    this.target.set(x, 0, z);
    this.pan(0, 0);
  }

  snap(): void {
    this.glide.copy(this.target);
    this.yaw = this.targetYaw;
    this.dist = this.targetDist;
  }

  zoom(factor: number): void {
    this.targetDist = THREE.MathUtils.clamp(
      this.targetDist * factor,
      MIN_DIST,
      this.span * MAX_DIST_FACTOR,
    );
  }

  update(): void {
    this.yaw += (this.targetYaw - this.yaw) * 0.25;
    this.dist += (this.targetDist - this.dist) * ZOOM_GLIDE;
    this.glide.x += (this.target.x - this.glide.x) * FOLLOW;
    this.glide.z += (this.target.z - this.glide.z) * FOLLOW;
    const h = this.groundHeight(this.glide.x, this.glide.z);
    const offset = new THREE.Vector3(
      Math.sin(this.yaw) * Math.cos(PITCH),
      Math.sin(PITCH),
      Math.cos(this.yaw) * Math.cos(PITCH),
    ).multiplyScalar(this.dist);
    this.camera.position.set(this.glide.x + offset.x, h + offset.y, this.glide.z + offset.z);
    this.camera.lookAt(this.glide.x, h, this.glide.z);
  }
}
