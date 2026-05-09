import { describe, expect, it } from 'vitest';
import {
  ANNOTATION_COLORS,
  DEFAULT_ANNOTATION_COLOR,
  isAnnotationColor,
  asAnnotationColor,
  annotationBorderClass,
  annotationBarClass,
  annotationSwatchClass,
  annotationSwatchHex
} from './annotationColors';

describe('annotationColors', () => {
  it('exports the four colors in stable order', () => {
    expect(ANNOTATION_COLORS).toEqual(['yellow', 'blue', 'green', 'pink']);
  });

  it('defaults to yellow', () => {
    expect(DEFAULT_ANNOTATION_COLOR).toBe('yellow');
  });

  describe('isAnnotationColor', () => {
    it('accepts the four valid colors', () => {
      for (const c of ANNOTATION_COLORS) {
        expect(isAnnotationColor(c)).toBe(true);
      }
    });
    it('rejects unknown / null / undefined', () => {
      expect(isAnnotationColor('orange')).toBe(false);
      expect(isAnnotationColor(undefined)).toBe(false);
      expect(isAnnotationColor(null)).toBe(false);
      expect(isAnnotationColor('')).toBe(false);
    });
  });

  describe('asAnnotationColor', () => {
    it('passes through valid colors', () => {
      expect(asAnnotationColor('blue')).toBe('blue');
    });
    it('coerces invalid input to the default', () => {
      expect(asAnnotationColor('purple')).toBe(DEFAULT_ANNOTATION_COLOR);
      expect(asAnnotationColor(undefined)).toBe(DEFAULT_ANNOTATION_COLOR);
      expect(asAnnotationColor(null)).toBe(DEFAULT_ANNOTATION_COLOR);
    });
  });

  describe('class helpers', () => {
    it('borderClass returns the matching Tailwind class', () => {
      expect(annotationBorderClass('blue')).toBe('border-l-blue-400');
      expect(annotationBorderClass('green')).toBe('border-l-green-400');
      expect(annotationBorderClass('pink')).toBe('border-l-pink-400');
      expect(annotationBorderClass('yellow')).toBe('border-l-yellow-400');
    });

    it('borderClass falls back to yellow on unknown / undefined input', () => {
      expect(annotationBorderClass('weird')).toBe('border-l-yellow-400');
      expect(annotationBorderClass(undefined)).toBe('border-l-yellow-400');
    });

    it('barClass returns the matching bg class', () => {
      expect(annotationBarClass('pink')).toBe('bg-pink-400');
      expect(annotationBarClass(undefined)).toBe('bg-yellow-400');
    });

    it('swatchClass mixes bg-300 + text-black for picker contrast', () => {
      // Accept any of the four — main check is that text-black is included
      // so a future palette change still keeps the contrast guarantee.
      for (const c of ANNOTATION_COLORS) {
        const out = annotationSwatchClass(c);
        expect(out).toContain('text-black');
        expect(out).toMatch(/bg-(yellow|blue|green|pink)-300/);
      }
    });

    it('swatchHex returns a 7-char hex per color', () => {
      for (const c of ANNOTATION_COLORS) {
        const hex = annotationSwatchHex(c);
        expect(hex).toMatch(/^#[0-9a-f]{6}$/);
      }
    });
  });
});
