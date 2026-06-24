/**
 * Patch script for @cloudflare/vitest-pool-workers
 *
 * Fixes ByteString encoding errors when the project path contains non-ASCII characters
 * (e.g., Chinese characters in "D:\Project\多人网页游戏").
 *
 * Root cause: workerd's module fallback service uses HTTP headers for 301 redirects
 * and WebSocket handshake data. HTTP headers only support Latin-1 (0-255) characters,
 * but file paths with Chinese characters exceed this range, causing TypeError.
 *
 * Patches applied:
 * 1. pool/index.mjs - buildRedirectResponse: Latin-1 encode non-ASCII paths in Location header
 * 2. pool/index.mjs - handleModuleFallbackRequest: Decode Latin-1 back to UTF-8 for specifiers/referrers
 * 3. pool/index.mjs - load: Remove non-ASCII direct-load fallback (Latin-1 redirect handles it)
 * 4. pool/index.mjs - resolve: Handle CJS ESM shim suffix and non-absolute target paths
 * 5. pool/index.mjs - connectToMiniflareSocket: Base64-encode MF-Vitest-Worker-Data header
 * 6. pool/index.mjs - runnerWorker.modules: Pre-register cloudflare:test-internal and cloudflare:test
 * 7. worker/index.mjs: Base64-decode MF-Vitest-Worker-Data header
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');
const POOL_INDEX = path.join(ROOT, 'node_modules/@cloudflare/vitest-pool-workers/dist/pool/index.mjs');
const WORKER_INDEX = path.join(ROOT, 'node_modules/@cloudflare/vitest-pool-workers/dist/worker/index.mjs');

function patchFile(filePath, description, replacements) {
  if (!fs.existsSync(filePath)) {
    console.warn(`[patch-vitest-pool-workers] File not found: ${filePath}`);
    return false;
  }

  let content = fs.readFileSync(filePath, 'utf-8');
  let patched = false;

  for (const { search, replace, alreadyPatchedMarker } of replacements) {
    if (content.includes(alreadyPatchedMarker || replace)) {
      console.log(`[patch-vitest-pool-workers] Already patched: ${description} - ${search.slice(0, 60)}...`);
      continue;
    }
    if (!content.includes(search)) {
      console.warn(`[patch-vitest-pool-workers] Search string not found in ${filePath}: ${search.slice(0, 80)}...`);
      continue;
    }
    content = content.replace(search, replace);
    patched = true;
  }

  if (patched) {
    fs.writeFileSync(filePath, content, 'utf-8');
    console.log(`[patch-vitest-pool-workers] Patched: ${description}`);
  }
  return patched;
}

// ─── Patch pool/index.mjs ───

const poolPatches = [
  // 1. buildRedirectResponse: Add Latin-1 encoding for non-ASCII paths
  {
    search:
      `function buildRedirectResponse(filePath) {
\tif (isWindows && filePath[0] !== "/") filePath = \`/\${filePath}\`;
\treturn new Response(null, {
\t\tstatus: 301,
\t\theaders: { Location: filePath }
\t});
}`,
    replace:
      `function buildRedirectResponse(filePath) {
\tif (isWindows && filePath[0] !== "/") filePath = \`/\${filePath}\`;
\tif (/[^\x00-\\x7F]/.test(filePath)) {
\t\tconst encoded = Buffer.from(filePath, "utf-8").toString("latin1");
\t\treturn new Response(null, {
\t\t\tstatus: 301,
\t\t\theaders: { Location: encoded }
\t\t});
\t}
\treturn new Response(null, {
\t\tstatus: 301,
\t\theaders: { Location: filePath }
\t});
}`,
    alreadyPatchedMarker: 'Buffer.from(filePath, "utf-8").toString("latin1")',
  },

  // 2. load: Remove non-ASCII direct-load fallback, always use buildRedirectResponse
  {
    search:
      `\tif (target !== filePath) {
\t\tif (method === "require" && !specifier.startsWith("node:")) filePath += disableCjsEsmShimSuffix;
\t\tif (/[^\x00-\\x7F]/.test(filePath)) {
\t\t\tdebuglog(logBase, "direct-load (non-ascii path):", filePath);
\t\t\tnonAsciiModulePathMap.set(target, filePath);
\t\t} else {
\t\t\tdebuglog(logBase, "redirect:", filePath);
\t\t\treturn buildRedirectResponse(filePath);
\t\t}
\t}`,
    replace:
      `\tif (target !== filePath) {
\t\tif (method === "require" && !specifier.startsWith("node:")) filePath += disableCjsEsmShimSuffix;
\t\tdebuglog(logBase, "redirect:", filePath);
\t\treturn buildRedirectResponse(filePath);
\t}`,
    alreadyPatchedMarker: 'debuglog(logBase, "redirect:", filePath);\n\t\treturn buildRedirectResponse(filePath);\n\t}',
  },

  // 3. resolve: Handle CJS ESM shim suffix and non-absolute target paths
  {
    search:
      `\tlet filePath = maybeGetTargetFilePath(target, method === "require");
\tif (filePath !== void 0) return filePath;`,
    replace:
      `\tlet filePath = maybeGetTargetFilePath(target, method === "require");
\tif (filePath !== void 0 && filePath === target && !path.isAbsolute(target.replace(disableCjsEsmShimSuffix, ""))) {
\t\tfilePath = void 0;
\t}
\tif (filePath !== void 0) return filePath;`,
    alreadyPatchedMarker: 'filePath === target && !path.isAbsolute(target.replace(disableCjsEsmShimSuffix',
  },

  // 4. resolve: Add CJS ESM shim suffix handling
  {
    search:
      `\treturn viteResolve(vite, specifier, referrer, method === "require");
}
function buildRedirectResponse(filePath) {`,
    replace:
      `\tif (specifier.endsWith(disableCjsEsmShimSuffix)) {
\t\tconst specifierWithoutSuffix = specifier.slice(0, -disableCjsEsmShimSuffix.length);
\t\tconst resolved = await viteResolve(vite, specifierWithoutSuffix, referrer, method === "require");
\t\treturn resolved + disableCjsEsmShimSuffix;
\t}
\treturn viteResolve(vite, specifier, referrer, method === "require");
}
function buildRedirectResponse(filePath) {`,
    alreadyPatchedMarker: 'specifier.endsWith(disableCjsEsmShimSuffix))',
  },

  // 5. handleModuleFallbackRequest: Add Latin-1 decoding for specifiers/referrers
  {
    search:
      `\tif (isWindows) {
\t\tif (target[0] === "/") target = target.substring(1);
\t\tif (referrer[0] === "/") referrer = referrer.substring(1);
\t}
\tconst referrerDir = posixPath.dirname(referrer);`,
    replace:
      `\tif (isWindows) {
\t\tif (target[0] === "/") target = target.substring(1);
\t\tif (referrer[0] === "/") referrer = referrer.substring(1);
\t}
\tif (/[\\x80-\\xFF]/.test(target)) {
\t\ttry {
\t\t\tconst decoded = Buffer.from(target, "latin1").toString("utf-8");
\t\t\tconst reencoded = Buffer.from(decoded, "utf-8").toString("latin1");
\t\t\tif (reencoded === target) target = decoded;
\t\t} catch {}
\t}
\tif (/[\\x80-\\xFF]/.test(referrer)) {
\t\ttry {
\t\t\tconst decoded = Buffer.from(referrer, "latin1").toString("utf-8");
\t\t\tconst reencoded = Buffer.from(decoded, "utf-8").toString("latin1");
\t\t\tif (reencoded === referrer) referrer = decoded;
\t\t} catch {}
\t}
\tconst referrerDir = posixPath.dirname(referrer);`,
    alreadyPatchedMarker: 'Buffer.from(target, "latin1").toString("utf-8")',
  },

  // 6. connectToMiniflareSocket: Base64-encode MF-Vitest-Worker-Data header
  {
    search:
      `\tconst res = await (await mf.getDurableObjectNamespace(RUNNER_OBJECT_BINDING, workerName)).get("singleton").fetch("http://placeholder", { headers: {
\t\tUpgrade: "websocket",
\t\t"MF-Vitest-Worker-Data": structuredSerializableStringify({ cwd: process.cwd() })
\t} });`,
    replace:
      `\tconst cwdData = structuredSerializableStringify({ cwd: process.cwd() });
\tconst cwdBase64 = Buffer.from(cwdData, 'utf-8').toString('base64');
\tconst res = await (await mf.getDurableObjectNamespace(RUNNER_OBJECT_BINDING, workerName)).get("singleton").fetch("http://placeholder", { headers: {
\t\tUpgrade: "websocket",
\t\t"MF-Vitest-Worker-Data": cwdBase64
\t} });`,
    alreadyPatchedMarker: "Buffer.from(cwdData, 'utf-8').toString('base64')",
  },

  // 7. runnerWorker.modules: Pre-register cloudflare:test-internal and cloudflare:test
  {
    search:
      `\t\t{
\t\t\ttype: "ESModule",
\t\t\tpath: path.join(modulesRoot, "__VITEST_POOL_WORKERS_DEFINES"),
\t\t\tcontents: defines
\t\t},
\t\t{
\t\t\ttype: "ESModule",
\t\t\tpath: path.join(modulesRoot, "node:console"),`,
    replace:
      `\t\t{
\t\t\ttype: "ESModule",
\t\t\tpath: path.join(modulesRoot, "__VITEST_POOL_WORKERS_DEFINES"),
\t\t\tcontents: defines
\t\t},
\t\t{
\t\t\ttype: "ESModule",
\t\t\tpath: path.join(modulesRoot, "cloudflare:test-internal"),
\t\t\tcontents: fs.readFileSync(path.join(DIST_PATH, \`worker/lib/cloudflare/test-internal.mjs\`))
\t\t},
\t\t{
\t\t\ttype: "ESModule",
\t\t\tpath: path.join(modulesRoot, "cloudflare:test"),
\t\t\tcontents: fs.readFileSync(path.join(DIST_PATH, \`worker/lib/cloudflare/test.mjs\`))
\t\t},
\t\t{
\t\t\ttype: "ESModule",
\t\t\tpath: path.join(modulesRoot, "node:console"),`,
    alreadyPatchedMarker: 'path.join(modulesRoot, "cloudflare:test-internal")',
  },
];

// ─── Patch worker/index.mjs ───

const workerPatches = [
  // Base64-decode MF-Vitest-Worker-Data header
  {
    search:
      `\tconst workerDataHeader = request.headers.get("MF-Vitest-Worker-Data");
\tassert(workerDataHeader);
\tconst wd = structuredSerializableParse(workerDataHeader);`,
    replace:
      `\tconst workerDataHeader = request.headers.get("MF-Vitest-Worker-Data");
\tassert(workerDataHeader);
\tlet decodedHeader = workerDataHeader;
\ttry { decodedHeader = new TextDecoder().decode(Uint8Array.from(atob(workerDataHeader), c => c.charCodeAt(0))); } catch {}
\tconst wd = structuredSerializableParse(decodedHeader);`,
    alreadyPatchedMarker: 'Uint8Array.from(atob(workerDataHeader)',
  },
];

// ─── Apply patches ───

console.log('[patch-vitest-pool-workers] Applying patches for non-ASCII path support...');
patchFile(POOL_INDEX, 'pool/index.mjs', poolPatches);
patchFile(WORKER_INDEX, 'worker/index.mjs', workerPatches);
console.log('[patch-vitest-pool-workers] Done.');
