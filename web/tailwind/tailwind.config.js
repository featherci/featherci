/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./web/templates/**/*.html",
    "./internal/server/routes.go", // Temporary: for inline HTML during development
  ],
  // Safelist component classes that are defined in @layer components
  // These need to be safelisted since they're defined with @apply and
  // won't be detected by the content scanner until used in templates
  safelist: [
    // Buttons
    "btn",
    "btn-primary",
    "btn-secondary",
    "btn-danger",
    "btn-sm",
    "btn-lg",
    "btn-github",
    "btn-gitlab",
    "btn-gitea",
    "btn-dev",
    // Form elements
    "input",
    "input-error",
    // Cards
    "card",
    "card-header",
    "card-body",
    "card-footer",
    // Badges
    "badge",
    "badge-success",
    "badge-failure",
    "badge-pending",
    "badge-running",
    "badge-cancelled",
    "badge-waiting",
    // Navigation
    "nav-link",
    "nav-link-active",
    // Tables
    "table",
    // Alerts
    "alert",
    "alert-info",
    "alert-success",
    "alert-warning",
    "alert-error",
    // Code
    "code",
    "code-block",
  ],
  theme: {
    extend: {
      colors: {
        // Custom brand colors - sky blue theme
        feather: {
          50: "#f0f9ff",
          100: "#e0f2fe",
          200: "#bae6fd",
          300: "#7dd3fc",
          400: "#38bdf8",
          500: "#0ea5e9",
          600: "#0284c7",
          700: "#0369a1",
          800: "#075985",
          900: "#0c4a6e",
          950: "#082f49",
        },
      },
    },
  },
  plugins: [],
};
