import type { Config } from 'tailwindcss';

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        brand: {
          50: '#eef5ff', 100: '#dbe8ff', 200: '#b8d0ff', 300: '#8bb0ff',
          400: '#5c8bff', 500: '#3b6dff', 600: '#2453e6', 700: '#1e41b3',
          800: '#1d3890', 900: '#1e3273',
        },
      },
      fontFamily: {
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
    },
  },
  plugins: [],
} satisfies Config;
