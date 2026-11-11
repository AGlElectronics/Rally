import { describe, expect, it } from 'vitest';
import { isDenPairingRequired, isDenRevoked } from './useDenState';

describe('Den state', () => {
  it('reports a revoked credential only for a paired module', () => {
    expect(isDenRevoked({ mode: 'standalone', paired: true, revoked: true })).toBe(true);
    expect(isDenRevoked({ mode: 'required', paired: true, revoked: false })).toBe(false);
    expect(isDenRevoked({ mode: 'required', paired: false, revoked: true })).toBe(false);
    expect(isDenRevoked({ mode: 'standalone', paired: true })).toBe(false);
    expect(isDenRevoked(undefined)).toBe(false);
  });

  it('keeps a revoked module unlocked (it still has its last bundle)', () => {
    expect(isDenPairingRequired({ mode: 'required', paired: true, revoked: true })).toBe(false);
    expect(isDenPairingRequired({ mode: 'required', paired: false, revoked: false })).toBe(true);
  });
});
