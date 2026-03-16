/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{svelte,js,ts}'],
  theme: {
    extend: {
      colors: {
        ctp: {
          base: 'var(--ctp-base)',
          mantle: 'var(--ctp-mantle)',
          crust: 'var(--ctp-crust)',
          surface0: 'var(--ctp-surface0)',
          surface1: 'var(--ctp-surface1)',
          surface2: 'var(--ctp-surface2)',
          overlay0: 'var(--ctp-overlay0)',
          overlay1: 'var(--ctp-overlay1)',
          text: 'var(--ctp-text)',
          subtext0: 'var(--ctp-subtext0)',
          subtext1: 'var(--ctp-subtext1)',
          blue: 'var(--ctp-blue)',
          lavender: 'var(--ctp-lavender)',
          sapphire: 'var(--ctp-sapphire)',
          sky: 'var(--ctp-sky)',
          teal: 'var(--ctp-teal)',
          green: 'var(--ctp-green)',
          yellow: 'var(--ctp-yellow)',
          peach: 'var(--ctp-peach)',
          maroon: 'var(--ctp-maroon)',
          red: 'var(--ctp-red)',
          mauve: 'var(--ctp-mauve)',
          pink: 'var(--ctp-pink)',
          flamingo: 'var(--ctp-flamingo)',
          rosewater: 'var(--ctp-rosewater)',
        },
      },
    },
  },
  plugins: [require('@tailwindcss/typography')],
}
