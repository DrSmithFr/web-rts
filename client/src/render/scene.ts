import * as THREE from 'three';

// Scene boot: gradient sky, one directional light with a shadow map, and
// the renderer sized to the window.

export interface SceneBundle {
  renderer: THREE.WebGLRenderer;
  scene: THREE.Scene;
  camera: THREE.PerspectiveCamera;
}

function skyTexture(): THREE.CanvasTexture {
  const canvas = document.createElement('canvas');
  canvas.width = 2;
  canvas.height = 128;
  const ctx = canvas.getContext('2d')!;
  const gradient = ctx.createLinearGradient(0, 0, 0, 128);
  gradient.addColorStop(0, '#7fb2e5');
  gradient.addColorStop(0.6, '#b8d4ef');
  gradient.addColorStop(1, '#dfe9f2');
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, 2, 128);
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

  const scene = new THREE.Scene();
  scene.background = skyTexture();
  scene.fog = new THREE.Fog(0xdfe9f2, mapSpan * 0.9, mapSpan * 2.2);

  const camera = new THREE.PerspectiveCamera(
    50,
    window.innerWidth / window.innerHeight,
    0.5,
    mapSpan * 4,
  );

  const sun = new THREE.DirectionalLight(0xfff2df, 1.6);
  sun.position.set(mapSpan * 0.7, mapSpan * 1.1, mapSpan * 0.35);
  sun.castShadow = true;
  sun.shadow.mapSize.set(2048, 2048);
  const extent = mapSpan * 0.75;
  sun.shadow.camera.left = -extent;
  sun.shadow.camera.right = extent;
  sun.shadow.camera.top = extent;
  sun.shadow.camera.bottom = -extent;
  sun.shadow.camera.far = mapSpan * 3;
  scene.add(sun);
  scene.add(new THREE.HemisphereLight(0xcfe6ff, 0x40502e, 0.8));

  window.addEventListener('resize', () => {
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
    renderer.setSize(window.innerWidth, window.innerHeight);
  });

  return { renderer, scene, camera };
}
