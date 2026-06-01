// Empirical proof that the index.css fix removes the CSP console-error
// class. Serves the freshly-built dist/ under the EXACT staging CSP
// header (copied from deploy/caddy/Caddyfile.staging + apps/api/internal/
// http/csp.go) and loads it in chromium, recording console errors.
//
// Control: also serve a synthetic page that still carries the old
// cross-origin @import, under the same CSP, to demonstrate the header
// genuinely blocks it (i.e. the test can detect the failure it claims
// to have fixed).
// Run inside the playwright container (unlimited vmem — the host shell's
// 6GB `ulimit -v` SIGTRAPs chromium): see DEMO-WALKTHROUGH.md "Reproduce".
import pw from 'playwright';
import http from 'node:http';
import { readFileSync, existsSync } from 'node:fs';
import { join, extname } from 'node:path';

const { chromium } = pw;
const CSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'";
const DIST = '/work/apps/web/dist';
const MIME = { '.html': 'text/html', '.css': 'text/css', '.js': 'text/javascript', '.svg': 'image/svg+xml', '.json': 'application/json' };

// Synthetic "old" page: the pre-fix @import, same CSP → must still error.
const OLD_PAGE = `<!doctype html><html><head>
<style>@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');</style>
</head><body>old</body></html>`;

const server = http.createServer((req, res) => {
  res.setHeader('Content-Security-Policy', CSP);
  if (req.url === '/__old__') {
    res.setHeader('Content-Type', 'text/html');
    return res.end(OLD_PAGE);
  }
  let p = req.url.split('?')[0];
  if (p === '/' || !existsSync(join(DIST, p))) p = '/index.html'; // SPA fallback
  const file = join(DIST, p);
  try {
    const body = readFileSync(file);
    res.setHeader('Content-Type', MIME[extname(file)] || 'application/octet-stream');
    res.end(body);
  } catch {
    res.statusCode = 404; res.end('nf');
  }
});
await new Promise((r) => server.listen(8077, '127.0.0.1', r));

async function consoleErrorsFor(path) {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] });
  const page = await browser.newPage();
  const errs = [];
  page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });
  page.on('pageerror', (e) => errs.push('PAGEERR ' + e));
  await page.goto('http://127.0.0.1:8077' + path, { waitUntil: 'networkidle', timeout: 20000 }).catch(() => {});
  await page.waitForTimeout(1500);
  await browser.close();
  return errs;
}

const fixedErrs = await consoleErrorsFor('/');
const controlErrs = await consoleErrorsFor('/__old__');
server.close();

const fontCsp = (arr) => arr.filter((e) => /fonts\.googleapis|Content Security Policy/i.test(e));
console.log(JSON.stringify({
  fixed_bundle: { total_console_errors: fixedErrs.length, font_csp_errors: fontCsp(fixedErrs).length, sample: fixedErrs.slice(0, 5) },
  control_old_page: { total_console_errors: controlErrs.length, font_csp_errors: fontCsp(controlErrs).length, sample: controlErrs.slice(0, 2) },
}, null, 2));
