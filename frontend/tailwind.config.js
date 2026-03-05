/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{vue,js}',
  ],
  theme: {
    extend: {
      colors: {
        g: {
          50:  '#f8faf8',
          100: '#f0f4f0',
          200: '#e2e8e2',
          300: '#c5cfc5',
          400: '#94a394',
          500: '#6b7d6b',
          600: '#4d5e4d',
          700: '#374437',
          800: '#222e22',
          900: '#141c14',
        },
        lime: {
          DEFAULT: '#7AD400',
          dk:      '#5fa300',
          lt:      '#e4f9a6',
          'x-lt':  '#f4fce8',
        },
        forest: {
          DEFAULT: '#007934',
          dk:      '#005526',
          lt:      '#d4eddf',
        },
      },
    },
  },
  plugins: [],
}
