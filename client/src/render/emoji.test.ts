import { describe, expect, it } from 'vitest';

import { conditionGlyph, CONDITION_GLYPHS } from './emoji';

const glyphOf = (cond: number): string => CONDITION_GLYPHS[conditionGlyph(cond)].glyph;

describe('conditionGlyph', () => {
  it('shows nothing when no condition is set', () => {
    expect(conditionGlyph(0)).toBe(-1);
  });

  it('maps each condition to its own glyph', () => {
    expect(glyphOf(1)).toBe('😨'); // fear
    expect(glyphOf(2)).toBe('😱'); // terror
    expect(glyphOf(4)).toBe('😡'); // rage
  });

  it('paints the most urgent condition when several bite at once', () => {
    expect(glyphOf(1 | 2)).toBe('😱'); // terror over fear
    expect(glyphOf(1 | 4)).toBe('😡'); // rage over fear
    expect(glyphOf(1 | 2 | 4)).toBe('😱'); // terror over both
  });
});
