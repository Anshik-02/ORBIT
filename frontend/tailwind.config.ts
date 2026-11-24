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
          bg: "#F4F7FC",
          bgSecondary: "#EAF1FA",
          card: "#FFFFFF",
          text: "#17243A",
          muted: "#64748B",
          accent: "#0891D5",
          purple: "#7965E8",
          border: "#DCE5F0",
          success: "#10B981",
          warning: "#F59E0B",
          terminal: "#0B1120",
          terminalBorder: "#1E293B",
          terminalText: "#E2E8F0",
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
          "50%": { transform: "translateY(-8px)" },
        },
        glow: {
          "0%": { boxShadow: "0 0 15px rgba(8, 145, 213, 0.15)" },
          "100%": { boxShadow: "0 0 30px rgba(121, 101, 232, 0.25)" },
        },
      },
      backgroundImage: {
        "radial-light": "radial-gradient(circle at 50% 30%, rgba(8, 145, 213, 0.1) 0%, rgba(121, 101, 232, 0.05) 50%, transparent 80%)",
        "mesh-light": "radial-gradient(at 10% 10%, rgba(8, 145, 213, 0.08) 0px, transparent 50%), radial-gradient(at 90% 90%, rgba(121, 101, 232, 0.08) 0px, transparent 50%)",
      },
    },
  },
  plugins: [],
};
export default config;
