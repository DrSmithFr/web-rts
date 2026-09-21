// Control groups: Ctrl+digit binds the current selection, digit recalls
// it, and Ctrl+Shift+digit adds. Pure client-side input state — groups
// are just id lists, so they never touch the simulation hash. Dead or
// forgotten ids drop out silently on recall.

export class ControlGroups {
  private groups = new Map<number, number[]>();

  bind(slot: number, ids: number[]): void {
    if (ids.length === 0) {
      this.groups.delete(slot);
      return;
    }
    this.groups.set(slot, [...ids]);
  }

  addTo(slot: number, ids: number[]): void {
    const cur = this.groups.get(slot) ?? [];
    const next = [...cur];
    for (const id of ids) {
      if (!next.includes(id)) next.push(id);
    }
    if (next.length === 0) {
      this.groups.delete(slot);
      return;
    }
    this.groups.set(slot, next);
  }

  // recall returns the group ids that are still alive.
  recall(slot: number, alive: (id: number) => boolean): number[] {
    const ids = this.groups.get(slot);
    if (!ids) return [];
    const kept = ids.filter(alive);
    if (kept.length === 0) {
      this.groups.delete(slot);
      return [];
    }
    this.groups.set(slot, kept);
    return kept;
  }
}
