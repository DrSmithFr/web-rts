import { describe, expect, it } from 'vitest';
import { t } from './index';

describe('i18n', () => {
  it('resolves english reference strings', () => {
    expect(t('hud.tick')).toBe('tick');
    expect(t('hud.status.loading')).toBe('loading simulation…');
  });

  it('interpolates variables', () => {
    expect(t('hud.status.ready', { version: '0.1.0', seed: 24045 })).toBe(
      'simulation 0.1.0 ready — seed 24045',
    );
  });

  it('falls back to the key for unknown messages', () => {
    expect(t('nope.missing' as never)).toBe('nope.missing');
  });
});
