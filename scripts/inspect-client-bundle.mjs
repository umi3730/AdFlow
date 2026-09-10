// Run after "npm run build" in web/. Sizes describe emitted JavaScript,
// not observed browser transfer, execution time, or end-to-end latency.
import { readFile, readdir } from "node:fs/promises";
import { gzipSync } from "node:zlib";

const client = new URL("../web/dist/client/", import.meta.url);
const manifestURL = new URL("../web/dist/server/__vite_rsc_assets_manifest.js", import.meta.url);
const { default: manifest } = await import(manifestURL.href);
const reference = Object.values(manifest.clientReferenceDeps).find((value) =>
  value.js.some((file) => /\/adflow-console-[^/]+\.js$/.test(file)),
);
if (!reference) throw new Error("Build does not contain the AdFlow console reference.");

const chunks = await Promise.all(
  (await readdir(new URL("_next/static/chunks/", client)))
    .filter((file) => file.endsWith(".js"))
    .map(async (name) => {
      const content = await readFile(new URL("_next/static/chunks/" + name, client));
      return { name, bytes: content.length, gzipBytes: gzipSync(content).length };
    }),
);
const eagerNames = new Set(
  [...manifest.clientEntryDeps.js, ...reference.js].map((file) => file.split("/").at(-1)),
);
const eager = chunks.filter((file) => eagerNames.has(file.name));
const entry = chunks.find((file) => /^adflow-console-/.test(file.name));
const total = (key) => eager.reduce((sum, file) => sum + file[key], 0);
console.log(
  JSON.stringify(
    {
      consoleEntry: entry,
      eagerJavaScript: {
        bytes: total("bytes"),
        gzipBytes: total("gzipBytes"),
        files: eager.map((file) => file.name),
      },
      largestChunks: chunks.sort((a, b) => b.bytes - a.bytes).slice(0, 10),
      oversizedChunks: chunks.filter((file) => file.bytes > 500 * 1024).map((file) => file.name),
    },
    null,
    2,
  ),
);
