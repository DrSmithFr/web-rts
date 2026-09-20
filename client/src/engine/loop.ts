// Fixed-timestep driver: the simulation advances in whole 50 ms ticks
// (20 Hz) while rendering runs at the display refresh rate. Catch-up after
// a stall is capped so a backgrounded tab never fast-forwards the game.

export const SIM_HZ = 20;
export const SIM_TICK_MS = 1000 / SIM_HZ;
export const MAX_CATCHUP_TICKS = 5;

// TimeAccumulator is pure integer-free logic (float ms only inside the
// browser driver, never inside the simulation) so it can be unit tested.
export class TimeAccumulator {
  private acc = 0;

  constructor(
    private readonly tickMs: number,
    private readonly maxSteps: number,
  ) {}

  // advance folds elapsed milliseconds into the accumulator and returns
  // how many whole ticks to execute now (at most maxSteps). Stale time
  // beyond the cap is dropped, keeping one fractional remainder.
  advance(elapsedMs: number): number {
    if (elapsedMs > 0) {
      this.acc += elapsedMs;
    }
    let steps = 0;
    while (this.acc >= this.tickMs && steps < this.maxSteps) {
      this.acc -= this.tickMs;
      steps++;
    }
    if (steps >= this.maxSteps && this.acc > this.tickMs) {
      this.acc = this.acc % this.tickMs;
    }
    return steps;
  }
}

export interface FrameScheduler {
  request(cb: () => void): void;
}

// FixedStepLoop wires an accumulator to a frame source.
export class FixedStepLoop {
  private readonly acc = new TimeAccumulator(SIM_TICK_MS, MAX_CATCHUP_TICKS);
  private last = 0;
  private running = false;

  constructor(
    private readonly simTick: () => void,
    private readonly render: () => void,
    private readonly now: () => number,
    private readonly schedule: FrameScheduler = {
      request: (cb) => requestAnimationFrame(cb),
    },
  ) {}

  start(): void {
    if (this.running) {
      return;
    }
    this.running = true;
    this.last = this.now();
    this.tick();
  }

  stop(): void {
    this.running = false;
  }

  private tick(): void {
    if (!this.running) {
      return;
    }
    const now = this.now();
    const elapsed = Math.max(0, now - this.last);
    this.last = now;
    const steps = this.acc.advance(elapsed);
    for (let i = 0; i < steps; i++) {
      this.simTick();
    }
    this.render();
    this.schedule.request(() => this.tick());
  }
}
