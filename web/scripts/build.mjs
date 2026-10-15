// Bundles the app with esbuild: src/main.tsx -> dist/bundle.js, src/styles/main.css -> dist/styles.css
import * as esbuild from "esbuild";
import { cpSync, existsSync, mkdirSync, copyFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const watch = process.argv.includes("--watch");

function copyStatic() {
  mkdirSync(join(root, "dist"), { recursive: true });
  copyFileSync(join(root, "index.html"), join(root, "dist/index.html"));
  if (existsSync(join(root, "public"))) cpSync(join(root, "public"), join(root, "dist"), { recursive: true });
}

const jsOptions = {
  entryPoints: [join(root, "src/main.tsx")],
  bundle: true,
  outfile: join(root, "dist/bundle.js"),
  format: "iife",
  target: ["es2020"],
  jsx: "automatic",
  sourcemap: true,
  minify: !watch,
  loader: { ".svg": "text" },
  define: { "process.env.NODE_ENV": JSON.stringify(watch ? "development" : "production") },
  logLevel: "info",
};

const cssOptions = {
  entryPoints: [join(root, "src/styles/main.css")],
  bundle: true,
  outfile: join(root, "dist/styles.css"),
  minify: !watch,
  logLevel: "info",
};

copyStatic();
if (watch) {
  const a = await esbuild.context(jsOptions);
  const b = await esbuild.context(cssOptions);
  await Promise.all([a.watch(), b.watch()]);
  console.log("Watching for changes…");
} else {
  await Promise.all([esbuild.build(jsOptions), esbuild.build(cssOptions)]);
  console.log("Build complete -> dist/");
}
