import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

import { renderMarkdown } from "./markdown";

export interface RenderStaticFixture {
  notes: Array<{ id: string; title: string }>;
}

export interface RenderStaticResult {
  pages: number;
  status: "ready";
}

export function renderStaticFixture(fixture: RenderStaticFixture): RenderStaticResult {
  return {
    pages: fixture.notes.length,
    status: "ready",
  };
}

export interface PublishBundleNote {
  id: string;
  slug?: string;
  title: string;
  body: string;
  tags?: string[];
}

export interface PublishBundleAsset {
  path: string;
  content: string;
}

export interface PublishBundle {
  notes: PublishBundleNote[];
  manifest?: unknown;
  graph?: unknown;
  searchIndex?: unknown;
  assets?: PublishBundleAsset[];
}

export interface RenderStaticBundleResult {
  files: string[];
  pages: number;
}

export async function renderStaticBundle(bundle: PublishBundle, outDir: string): Promise<RenderStaticBundleResult> {
  const files: string[] = [];
  const noteSummaries = bundle.notes.map((note) => ({
    id: note.id,
    slug: noteSlug(note),
    title: note.title,
    tags: note.tags ?? [],
  }));

  await writeText(outDir, "index.html", pageShell("Pinax publish preview", noteListHTML(noteSummaries)), files);

  const tags = new Map<string, typeof noteSummaries>();
  for (const note of bundle.notes) {
    const rendered = await renderMarkdown(note.body);
    const slug = noteSlug(note);
    const noteHTML = `<article><h1>${escapeHTML(note.title)}</h1>${rendered.html}</article>`;
    await writeText(outDir, `notes/${slug}/index.html`, pageShell(note.title, noteHTML), files);
    for (const tag of note.tags ?? []) {
      const items = tags.get(tag) ?? [];
      items.push({ id: note.id, slug, title: note.title, tags: note.tags ?? [] });
      tags.set(tag, items);
    }
  }

  for (const [tag, notes] of tags) {
    await writeText(outDir, `tags/${slug(tag)}/index.html`, pageShell(`Tag: ${tag}`, noteListHTML(notes)), files);
  }

  await writeJSON(outDir, "pinax-data/manifest.json", bundle.manifest ?? { notes: noteSummaries }, files);
  await writeJSON(outDir, "pinax-data/graph.json", bundle.graph ?? { nodes: [], edges: [] }, files);
  await writeJSON(outDir, "pinax-data/search-index.json", bundle.searchIndex ?? noteSummaries, files);

  for (const asset of bundle.assets ?? []) {
    await writeText(outDir, asset.path, asset.content, files);
  }

  return { files: files.sort(), pages: bundle.notes.length + tags.size + 1 };
}

export async function renderStaticBundleFromRoot(bundleRoot: string, outDir: string): Promise<RenderStaticBundleResult> {
  const notesPayload = await readJSON<{ notes?: Array<Record<string, unknown>> }>(bundleRoot, "notes.json");
  const notes = (notesPayload.notes ?? []).map((note) => ({
    id: stringValue(note.id),
    slug: slugFromBundlePath(stringValue(note.path)),
    title: stringValue(note.title),
    body: stringValue(note.body),
    tags: Array.isArray(note.tags) ? note.tags.map((tag) => String(tag)) : [],
  }));
  const bundle: PublishBundle = {
    notes,
    manifest: await readOptionalJSON(bundleRoot, "manifest.json"),
    graph: await readOptionalJSON(bundleRoot, "graph.json"),
    searchIndex: await readOptionalJSON(bundleRoot, "search-index.json"),
    assets: await readBundleAssets(bundleRoot),
  };
  return renderStaticBundle(bundle, outDir);
}

async function readJSON<T>(root: string, rel: string): Promise<T> {
  return JSON.parse(await readFile(path.join(root, rel), "utf8")) as T;
}

async function readOptionalJSON(root: string, rel: string): Promise<unknown> {
  try {
    return await readJSON(root, rel);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return undefined;
    }
    throw error;
  }
}

async function readBundleAssets(bundleRoot: string): Promise<PublishBundleAsset[]> {
  const assetRoot = path.join(bundleRoot, "assets");
  const assets: PublishBundleAsset[] = [];
  async function walk(dir: string): Promise<void> {
    let entries;
    try {
      entries = await readdir(dir, { withFileTypes: true });
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ENOENT") {
        return;
      }
      throw error;
    }
    for (const entry of entries) {
      const abs = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        await walk(abs);
        continue;
      }
      if (!entry.isFile()) {
        continue;
      }
      const rel = path.relative(bundleRoot, abs).split(path.sep).join("/");
      assets.push({ path: rel, content: await readFile(abs, "utf8") });
    }
  }
  await walk(assetRoot);
  return assets;
}

async function writeJSON(outDir: string, rel: string, value: unknown, files: string[]): Promise<void> {
  await writeText(outDir, rel, `${JSON.stringify(value, null, 2)}\n`, files);
}

async function writeText(outDir: string, rel: string, content: string, files: string[]): Promise<void> {
  const safeRel = safeRelativePath(rel);
  const target = path.join(outDir, safeRel);
  await mkdir(path.dirname(target), { recursive: true });
  await writeFile(target, content);
  files.push(safeRel);
}

function safeRelativePath(rel: string): string {
  const normalized = rel.split(path.sep).join("/");
  if (path.isAbsolute(rel) || normalized === ".." || normalized.startsWith("../") || normalized.includes("/../") || normalized === ".pinax" || normalized.startsWith(".pinax/")) {
    throw new Error(`unsafe publish renderer output path: ${rel}`);
  }
  return normalized;
}

function noteSlug(note: PublishBundleNote): string {
  return note.slug ? slug(note.slug) : slug(note.id || note.title);
}

function slugFromBundlePath(value: string): string {
  const trimmed = value.replace(/^notes\//, "").replace(/\/$/, "");
  return trimmed || value;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function slug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

function pageShell(title: string, body: string): string {
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>${escapeHTML(title)}</title></head><body>${body}</body></html>`;
}

function noteListHTML(notes: Array<{ slug: string; title: string }>): string {
  const items = notes.map((note) => `<li><a href="/notes/${escapeAttribute(note.slug)}/">${escapeHTML(note.title)}</a></li>`).join("");
  return `<main><h1>Pinax publish preview</h1><ul>${items}</ul></main>`;
}

function escapeHTML(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

function escapeAttribute(value: string): string {
  return escapeHTML(value).replaceAll("'", "&#39;");
}

function argValue(args: string[], name: string): string {
  const index = args.indexOf(name);
  return index >= 0 ? (args[index + 1] ?? "") : "";
}

if (import.meta.main) {
  const bundleRoot = argValue(Bun.argv, "--bundle");
  const outDir = argValue(Bun.argv, "--out");
  const result = bundleRoot && outDir ? await renderStaticBundleFromRoot(bundleRoot, outDir) : renderStaticFixture({ notes: [] });
  console.log(JSON.stringify(result));
}
