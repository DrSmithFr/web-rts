import * as THREE from 'three';

// Scene boot: gradient sky, one directional light whose shadow camera
// follows the view (sharp shadows at any zoom), hemisphere fill and the
// renderer sized to the window with filmic tone mapping.

export interface SceneBundle {
  renderer: THREE.WebGLRenderer;
  scene: THREE.Scene;
  camera: THREE.PerspectiveCamera;
  sun: THREE.DirectionalLight;
  hemi: THREE.HemisphereLight;
  sky: THREE.CanvasTexture;
}

function skyTexture(): THREE.CanvasTexture {
  const canvas = document.createElement('canvas');
  canvas.width = 2;
  canvas.height = 256;
  const ctx = canvas.getContext('2d')!;
  const gradient = ctx.createLinearGradient(0, 0, 0, 256);
  gradient.addColorStop(0, '#5f93cf');
  gradient.addColorStop(0.55, '#a8c8e8');
  gradient.addColorStop(0.85, '#e2d9b8');
  gradient.addColorStop(1, '#f0e3c2');
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, 2, 256);
  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  return texture;
}

export function createScene(canvas: HTMLCanvasElement, mapSpan: number): SceneBundle {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  renderer.setSize(window.innerWidth, window.innerHeight);
  renderer.shadowMap.enabled = true;
  renderer.shadowMap.type = THREE.PCFShadowMap;
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.12;

  const scene = new THREE.Scene();
  const sky = skyTexture();
  scene.background = sky;
  scene.fog = new THREE.Fog(0xe8ddc2, mapSpan * 1.0, mapSpan * 2.6);

  const camera = new THREE.PerspectiveCamera(
    50,
    window.innerWidth / window.innerHeight,
    0.5,
    mapSpan * 4,
  );

  const sun = new THREE.DirectionalLight(0xfff0d8, 2.1);
  sun.castShadow = true;
  sun.shadow.mapSize.set(2048, 2048);
  sun.shadow.bias = -0.0004;
  sun.shadow.normalBias = 0.02;
  const extent = mapSpan * 0.4;
  sun.shadow.camera.left = -extent;
  sun.shadow.camera.right = extent;
  sun.shadow.camera.top = extent;
  sun.shadow.camera.bottom = -extent;
  sun.shadow.camera.far = mapSpan * 3;
  scene.add(sun);
  scene.add(sun.target);
  const hemi = new THREE.HemisphereLight(0xcfe6ff, 0x46512e, 0.95);
  scene.add(hemi);

  window.addEventListener('resize', () => {
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
    renderer.setSize(window.innerWidth, window.innerHeight);
  });

  return { renderer, scene, camera, sun, hemi, sky };
}

// frameShadow pins the shadow frustum around the view so its resolution
// does not dilute across the whole map. Returns whether it changed.
let lastExtent = -1;

export function frameShadow(
  bundle: SceneBundle,
  tx: number,
  tz: number,
  extent: number,
  sunDir: THREE.Vector3,
): void {
  const sun = bundle.sun;
  sun.target.position.set(tx, 0, tz);
  sun.position.copy(sun.target.position).addScaledVector(sunDir, extent * 2.4);
  sun.target.updateMatrixWorld();
  if (Math.abs(extent - lastExtent) > 2) {
    lastExtent = extent;
    const c = sun.shadow.camera;
    c.left = -extent;
    c.right = extent;
    c.top = extent;
    c.bottom = -extent;
    c.far = extent * 6;
    c.updateProjectionMatrix();
  }
}
