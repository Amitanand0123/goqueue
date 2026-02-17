/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./web/templates/**/*.html"],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        dark: { 700: "#252547", 800: "#1a1a2e", 900: "#0f0f23" },
      },
    },
  },
  plugins: [],
};
