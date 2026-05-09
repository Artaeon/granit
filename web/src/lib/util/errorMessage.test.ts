import { describe, expect, it } from 'vitest';
import { errorMessage } from './errorMessage';

describe('errorMessage', () => {
  it('extracts the message from an Error', () => {
    expect(errorMessage(new Error('boom'))).toBe('boom');
  });

  it('returns subclass messages too', () => {
    expect(errorMessage(new TypeError('bad type'))).toBe('bad type');
  });

  it('stringifies a plain string', () => {
    expect(errorMessage('plain')).toBe('plain');
  });

  it('stringifies undefined / null without crashing', () => {
    expect(errorMessage(undefined)).toBe('undefined');
    expect(errorMessage(null)).toBe('null');
  });

  it('stringifies arbitrary thrown objects', () => {
    expect(errorMessage({ status: 500 })).toBe('[object Object]');
    expect(errorMessage(42)).toBe('42');
  });
});
