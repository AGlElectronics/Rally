import { defineConfig } from '@rsbuild/core';
import { pluginReact } from '@rsbuild/plugin-react';

// Rsbuild (like Vite) does not auto-forward process.env into import.meta.env.
// Explicitly map the VITE_* vars Rally reads so Docker/CI must set them
// before `rsbuild build` runs — a typo surfaces at build time, not runtime.
const viteEnv = {
  VITE_AUTH_MODE: process.env.VITE_AUTH_MODE ?? 'dev',
  VITE_OIDC_AUTHORITY: process.env.VITE_OIDC_AUTHORITY ?? '',
  VITE_OIDC_CLIENT_ID: process.env.VITE_OIDC_CLIENT_ID ?? '',
  VITE_OIDC_SCOPE: process.env.VITE_OIDC_SCOPE ?? 'openid profile email',
  VITE_API_BASE_URL: process.env.VITE_API_BASE_URL ?? '',
  VITE_PRODUCT_NAME: process.env.VITE_PRODUCT_NAME ?? 'Rally',
} as const;

// One codebase, two apps: the staff app (default) and the public submission
// site (RALLY_APP=submit), which is built and hosted separately.
const submitApp = process.env.RALLY_APP === 'submit';

export default defineConfig({
  plugins: [pluginReact()],
  source: {
    entry: { index: submitApp ? './src/submit/main.tsx' : './src/main.tsx' },
    define: {
      'process.env.RALLY_VERSION': JSON.stringify(process.env.RALLY_VERSION ?? 'dev'),
      'import.meta.env.VITE_AUTH_MODE': JSON.stringify(viteEnv.VITE_AUTH_MODE),
      'import.meta.env.VITE_OIDC_AUTHORITY': JSON.stringify(viteEnv.VITE_OIDC_AUTHORITY),
      'import.meta.env.VITE_OIDC_CLIENT_ID': JSON.stringify(viteEnv.VITE_OIDC_CLIENT_ID),
      'import.meta.env.VITE_OIDC_SCOPE': JSON.stringify(viteEnv.VITE_OIDC_SCOPE),
      'import.meta.env.VITE_API_BASE_URL': JSON.stringify(viteEnv.VITE_API_BASE_URL),
      'import.meta.env.VITE_PRODUCT_NAME': JSON.stringify(viteEnv.VITE_PRODUCT_NAME),
    },
  },
  resolve: {
    alias: {
      '@': './src',
    },
  },
  html: {
    template: './public/index.html',
    title: submitApp ? viteEnv.VITE_PRODUCT_NAME : 'Rally — Events',
    meta: {
      description: submitApp ? 'Submit your event idea' : 'Rally events & convention management',
      'theme-color': '#18181b',
    },
  },
  server: {
    // Rally frontend runs on 3002 to avoid colliding with Stash (3000)
    // and the central-manager scaffold's reserved 3001.
    port: submitApp ? 3004 : 3002,
    proxy: {
      '/api': {
        target: process.env.API_PROXY_TARGET ?? 'http://localhost:8100',
        changeOrigin: true,
      },
    },
  },
  output: {
    distPath: { root: 'dist' },
    cleanDistPath: true,
    copy: [
      { from: 'public/brand.json', to: 'brand.json' },
      { from: 'public/manifest.webmanifest', to: 'manifest.webmanifest' },
    ],
  },
});
