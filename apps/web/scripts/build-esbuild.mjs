// Fallback SPA build using esbuild (native) + the Tailwind CLI (pure JS).
//
// The canonical build is `vite build`. This fallback exists for build
// environments where Vite's bundler can't run — notably hosts with a tight
// RLIMIT_AS (virtual-memory ulimit), under which V8's WebAssembly engine
// (used by vite's es-module-lexer) fails to reserve its address space.
// esbuild is a native Go binary and the Tailwind CLI is plain JS, so
// neither touches WebAssembly.
//
// Output mirrors Vite's layout closely enough for the Go go:embed handler:
//   dist/index.html              -> SPA shell (script + stylesheet links)
//   dist/assets/app.js           -> bundled, minified ESM
//   dist/assets/app.css          -> Tailwind-compiled, minified CSS
//
// Usage:  node scripts/build-esbuild.mjs
import { build } from 'esbuild';
import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync, rmSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const dist = path.join(root, 'dist');
const assets = path.join(dist, 'assets');

rmSync(dist, { recursive: true, force: true });
mkdirSync(assets, { recursive: true });

// 1. Tailwind CSS (processes @tailwind directives, scans content globs).
console.log('==> compiling Tailwind CSS');
execFileSync(
  process.execPath,
  [
    path.join(root, 'node_modules/tailwindcss/lib/cli.js'),
    '-i', path.join(root, 'src/index.css'),
    '-o', path.join(assets, 'app.css'),
    '--minify',
  ],
  { cwd: root, stdio: 'inherit' },
);

// 2. esbuild bundle. A tiny plugin swallows the `./index.css` import — the
//    CSS is delivered by the Tailwind step above and linked from index.html.
const ignoreCss = {
  name: 'ignore-css',
  setup(b) {
    b.onResolve({ filter: /\.css$/ }, (args) => ({
      path: args.path,
      namespace: 'ignore-css',
    }));
    b.onLoad({ filter: /.*/, namespace: 'ignore-css' }, () => ({ contents: '' }));
  },
};

console.log('==> bundling app with esbuild');
await build({
  entryPoints: [path.join(root, 'src/main.tsx')],
  bundle: true,
  format: 'esm',
  target: 'es2022',
  jsx: 'automatic',
  minify: true,
  sourcemap: false,
  outfile: path.join(assets, 'app.js'),
  define: { 'process.env.NODE_ENV': '"production"' },
  alias: { '@': path.join(root, 'src') },
  loader: { '.svg': 'dataurl' },
  logLevel: 'info',
  plugins: [ignoreCss],
});

// 3. SPA shell.
writeFileSync(
  path.join(dist, 'index.html'),
  `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>leCRM</title>
    <link rel="stylesheet" href="/assets/app.css" />
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/assets/app.js"></script>
  </body>
</html>
`,
);

console.log('==> esbuild SPA build complete ->', dist);
