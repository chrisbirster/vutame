import { gzipSync } from "node:zlib";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const ROOT = "internal/web/dist";
const budgets = {
  js: 220 * 1024,
  css: 80 * 1024,
  total: 280 * 1024,
};

const files = walk(ROOT).filter((file) => [".js", ".css"].includes(extname(file)));
let js = 0;
let css = 0;
const rows = [];

for (const file of files) {
  const gzipBytes = gzipSync(readFileSync(file), { level: 9 }).byteLength;
  rows.push({ file: relative(ROOT, file), gzipBytes });
  if (extname(file) === ".js") js += gzipBytes;
  if (extname(file) === ".css") css += gzipBytes;
}

const total = js + css;
console.log("Vutame mobile web budget (gzip)");
for (const row of rows.sort((a, b) => b.gzipBytes - a.gzipBytes)) {
  console.log(`${format(row.gzipBytes).padStart(9)}  ${row.file}`);
}
console.log(`${format(js).padStart(9)}  JavaScript total / ${format(budgets.js)}`);
console.log(`${format(css).padStart(9)}  CSS total / ${format(budgets.css)}`);
console.log(`${format(total).padStart(9)}  JS + CSS total / ${format(budgets.total)}`);

const failures = [];
if (js > budgets.js) failures.push(`JavaScript ${format(js)} exceeds ${format(budgets.js)}`);
if (css > budgets.css) failures.push(`CSS ${format(css)} exceeds ${format(budgets.css)}`);
if (total > budgets.total) failures.push(`combined ${format(total)} exceeds ${format(budgets.total)}`);
if (failures.length > 0) {
  console.error(`Web budget failed: ${failures.join("; ")}`);
  process.exit(1);
}

function walk(directory) {
  const output = [];
  for (const entry of readdirSync(directory)) {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) output.push(...walk(path));
    else output.push(path);
  }
  return output;
}

function format(bytes) {
  return `${(bytes / 1024).toFixed(1)} KiB`;
}
