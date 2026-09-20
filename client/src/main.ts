import { loadSim } from './sim/loader';

const status = document.querySelector<HTMLDivElement>('#status')!;

status.textContent = 'loading simulation…';
const sim = await loadSim();
status.textContent = `sim ${sim.version()} loaded`;
