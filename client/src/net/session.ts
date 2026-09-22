// NetSession: browser client for the authoritative multiplayer server.
// The server owns the tick clock and the order batches; this session
// steps the local world exactly once per server tick, verifies the state
// hash and confirms it back. Order payloads are sent to the server and
// come back inside tick batches, so every peer applies the same stream.

export interface TickOrder {
  player: number;
  kind: number;
  payload: Uint8Array;
}

export interface Boot {
  seed: string;
  player: number;
  playerCount: number;
  tick: number;
  orders: Map<number, TickOrder[]>;
}

export interface LobbyInfo {
  slot: number;
  name: string;
  color: number;
  bot?: boolean;
  online: boolean;
}

export interface SessionHandlers {
  onStatus(msg: string): void;
  onLobby(code: string, players: LobbyInfo[]): void;
}

function b64decode(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function b64encode(b: Uint8Array): string {
  let s = '';
  for (let i = 0; i < b.length; i++) s += String.fromCharCode(b[i]);
  return btoa(s);
}

export class NetSession {
  private ws: WebSocket;
  private pending: ((b: Boot) => void) | null = null;
  bootData: Boot | null = null;
  active = false;
  myPlayer = 0;
  code = '';
  private tickQueue: { tick: number; orders: TickOrder[]; hashHex: string }[] = [];
  private tickSink: ((tick: number, orders: TickOrder[], hashHex: string) => void) | null = null;

  set onTick(fn: ((tick: number, orders: TickOrder[], hashHex: string) => void) | null) {
    this.tickSink = fn;
    if (fn) {
      const q = this.tickQueue;
      this.tickQueue = [];
      let expect = q.length ? q[0]?.tick ?? 0 : 0;
      for (const t of q) {
        if (t.tick !== expect) console.warn(`net: tick gap ${expect} -> ${t.tick}`);
        expect = t.tick + 1;
        fn(t.tick, t.orders, t.hashHex);
      }
    }
  }

  constructor(
    url: string,
    private handlers: SessionHandlers,
  ) {
    const path = new URL(url).pathname;
    const full = path === '/' || path === '' ? url.replace(/\/$/, '') + '/ws' : url;
    this.ws = new WebSocket(full);
    this.ws.onmessage = (ev) => this.handle(JSON.parse(ev.data as string));
    this.ws.onopen = () => this.handlers.onStatus('connected');
    this.ws.onclose = () => {
      this.active = false;
      this.handlers.onStatus('disconnected');
    };
  }

  create(name: string, bots: number): void {
    this.send({ t: 'create', name, bots });
  }

  join(code: string, name: string, token: string): void {
    this.send({ t: 'join', code, name, token });
  }

  start(): void {
    this.send({ t: 'start' });
  }

  sendOrder(kind: number, payload: Uint8Array): void {
    this.send({ t: 'order', kind, payload: b64encode(payload) });
  }

  confirm(tick: number, hashHex: string): void {
    this.send({ t: 'hash', tick, hash: BigInt('0x' + hashHex).toString() });
  }

  booted(): Promise<Boot> {
    if (this.bootData) return Promise.resolve(this.bootData);
    return new Promise((resolve) => {
      this.pending = resolve;
    });
  }

  private send(msg: Record<string, unknown>): void {
    const wire = JSON.stringify(msg);
    if (this.ws.readyState === WebSocket.OPEN) this.ws.send(wire);
    else this.ws.addEventListener('open', () => this.ws.send(wire), { once: true });
  }

  private parseOrders(list: { tick: number; player: number; kind: number; payload?: string }[]): {
    tick: number;
    orders: TickOrder[];
  }[] {
    if (!list) return [];
    const grouped = new Map<number, TickOrder[]>();
    for (const o of list) {
      const arr = grouped.get(o.tick) ?? [];
      arr.push({ player: o.player, kind: o.kind, payload: o.payload ? b64decode(o.payload) : new Uint8Array() });
      grouped.set(o.tick, arr);
    }
    return [...grouped.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([tick, orders]) => ({ tick, orders }));
  }

  private handle(m: {
    t: string;
    code?: string;
    player?: number;
    token?: string;
    seed?: string;
    tick?: number;
    hash?: string;
    players?: LobbyInfo[];
    orders?: { tick: number; player: number; kind: number; payload?: string }[];
    msg?: string;
    reason?: string;
  }): void {
    switch (m.t) {
      case 'welcome': {
        this.myPlayer = m.player ?? 0;
        this.code = m.code ?? '';
        if (m.token) sessionStorage.setItem('rts.token', m.token);
        if (m.code) sessionStorage.setItem('rts.code', m.code);
        if (m.seed !== undefined && m.tick !== undefined) {
          const orders = new Map<number, TickOrder[]>();
          for (const g of this.parseOrders(m.orders ?? [])) orders.set(g.tick, g.orders);
          this.bootData = {
            seed: m.seed, player: this.myPlayer, playerCount: m.players?.length ?? 2,
            tick: m.tick, orders,
          };
          this.active = true;
          this.resolveBoot();
        }
        break;
      }
      case 'start': {
        this.bootData = {
          seed: m.seed ?? '5eed', player: this.myPlayer, playerCount: m.players?.length ?? 2,
          tick: 0, orders: new Map(),
        };
        this.active = true;
        this.resolveBoot();
        break;
      }
      case 'lobby':
        this.handlers.onLobby(m.code ?? '', m.players ?? []);
        break;
      case 'tick': {
        if (m.tick === undefined) break;
        const orders = this.parseOrders(m.orders ?? []);
        let batch: TickOrder[] = [];
        for (const g of orders) batch = batch.concat(g.orders);
        if (this.tickSink) this.tickSink(m.tick, batch, m.hash ?? '');
        else this.tickQueue.push({ tick: m.tick, orders: batch, hashHex: m.hash ?? '' });
        break;
      }
      case 'pause':
        this.active = false;
        this.handlers.onStatus(m.reason === 'latency' ? 'paused: latency' : 'paused');
        break;
      case 'resume':
        this.active = true;
        this.handlers.onStatus('running');
        break;
      case 'desync':
        this.handlers.onStatus(`DESYNC ${m.msg ?? ''}`);
        break;
      case 'error':
        this.handlers.onStatus(`error: ${m.msg ?? '?'}`);
        break;
    }
  }

  private resolveBoot(): void {
    if (this.pending && this.bootData) {
      const b = this.bootData;
      const resolve = this.pending;
      this.pending = null;
      this.handlers.onStatus(`online: room ${this.code} as player ${this.myPlayer}`);
      resolve(b);
    }
  }
}
