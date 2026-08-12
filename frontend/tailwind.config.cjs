const { addDynamicIconSelectors } = require("@iconify/tailwind");

const iconSafelist = [
  "icon-[ant-design--bilibili-outlined]",
  "icon-[bxl--openai]",
  "icon-[cil--badge]",
  "icon-[dashicons--yes]",
  "icon-[ic--round-close]",
  "icon-[ic--round-minus]",
  "icon-[logos--claude-icon]",
  "icon-[mdi--api]",
  "icon-[mdi--brain]",
  "icon-[mdi--check]",
  "icon-[mdi--chevron-down]",
  "icon-[mdi--content-copy]",
  "icon-[mdi--eye-off-outline]",
  "icon-[mdi--eye-outline]",
  "icon-[mdi--file-document-outline]",
  "icon-[mdi--head-cog-outline]",
  "icon-[mdi--head-lightbulb-outline]",
  "icon-[mdi--head-outline]",
  "icon-[mdi--information-outline]",
  "icon-[mdi--message-text-outline]",
  "icon-[mdi--pause]",
  "icon-[mdi--play]",
  "icon-[mdi--refresh]",
  "icon-[mdi--wifi]",
  "icon-[mingcute--loading-fill]",
];

module.exports = {
  content: ["./index.html", "./src/**/*.{vue,js,jsx,ts,tsx}"],
  safelist: [...iconSafelist, "z-999", "z-9999", "z-99999"],
  theme: {
    extend: {
      colors: {
        // 品牌绿：主色 #10AD5D，用于焦点环/选中态/主按钮/状态
        brand: {
          300: "#5fd6a0",
          400: "#34d399",
          500: "#10AD5D",
          600: "#0b8a4b",
          700: "#086e3d",
          DEFAULT: "#10AD5D",
        },
        // 表面层级：page < card < input < hover
        surface: {
          page: "#121212",
          card: "#171717",
          input: "#1f1f1f",
          hover: "#262626",
          overlay: "#0f0f0f",
        },
        // 文字层级：primary(主) / secondary(次) / muted(弱)
        ink: {
          primary: "#e5e5e5",
          secondary: "#a1a1aa",
          muted: "#71717a",
        },
        // 分割线/边框：subtle < DEFAULT < strong
        line: {
          subtle: "#232323",
          DEFAULT: "#2a2a2a",
          strong: "#3a3a3a",
        },
      },
      fontFamily: {
        num: [
          "HFKos",
          "PingFang-Medium",
          "system-ui",
          "-apple-system",
          "BlinkMacSystemFont",
          "\"Segoe UI\"",
          "Roboto",
          "sans-serif",
        ],
      },
      fontSize: {
        xs: ["12px", { lineHeight: "16px" }],
        sm: ["13px", { lineHeight: "18px" }],
        lg: ["20px", { lineHeight: "28px" }],
      },
      boxShadow: {
        card: "0 1px 2px rgba(0, 0, 0, 0.4)",
        pop: "0 12px 32px rgba(0, 0, 0, 0.45)",
      },
      zIndex: {
        999: "999",
        9999: "9999",
        99999: "99999",
      },
    },
  },
  plugins: [addDynamicIconSelectors()],
};
