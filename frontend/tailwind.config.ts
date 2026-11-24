import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        orbit: {
          bg: "#030712",
          card: "rgba(10, 17, 40, 0.75)",
          border: "rgba(56, 189, 248, 0.15)",
          accent: "#38bdf8",
          purple: "#a855f7",
          green: "#10b981",
          amber: "#f59e0b",
          red: "#ef4444",
          surface: "#080e22",
        },
      },
      fontFamily: {
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "Monaco", "Consolas", "monospace"],
        sans: ["Switzer", "Inter", "-apple-system", "BlinkMacSystemFont", "Segoe UI", "Roboto", "sans-serif"],
      },
      animation: {
        "pulse-slow": "pulse 4s cubic-bezier(0.4, 0, 0.6, 1) infinite",
        "float": "float 6s ease-in-out infinite",
        "glow": "glow 2s ease-in-out infinite alternate",
        "spin-slow": "spin 20s linear infinite",
      },
      keyframes: {
        float: {
          "0%, 100%": { transform: "translateY(0px)" },
          "50%": { transform: "translateY(-10px)" },
        },
        glow: {
          "0%": { boxShadow: "0 0 15px rgba(56, 189, 248, 0.3)" },
          "100%": { boxShadow: "0 0 35px rgba(56, 189, 248, 0.7)" },
        },
      },
      backgroundImage: {
        "radial-gradient": "radial-gradient(circle at 50% 50%, rgba(56, 189, 248, 0.12), transparent 70%)",
        "mesh-gradient": "radial-gradient(at 0% 0%, rgba(56, 189, 248, 0.15) 0px, transparent 50%), radial-gradient(at 100% 100%, rgba(168, 85, 247, 0.15) 0px, transparent 50%)",
      },
    },
  },
  plugins: [],
};
export default config;
