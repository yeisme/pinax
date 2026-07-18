import { describe, expect, it } from "vitest";
import { mkdtemp, readFile, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { renderStaticBundle, renderStaticFixture } from "./render-static";

describe("renderStaticFixture", () => {
  it("accepts an empty publish fixture", () => {
    expect(renderStaticFixture({ notes: [] })).toEqual({ pages: 0, status: "ready" });
  });

  it("writes static pages, assets, and pinax-data projections", async () => {
    const outDir = await mkdtemp(path.join(tmpdir(), "pinax-render-static-"));
    const result = await renderStaticBundle(
      {
        notes: [
          {
            id: "note_alpha",
            slug: "alpha",
            title: "Alpha",
            tags: ["public"],
            body: "---\ntitle: Alpha\n---\n\n# Alpha\n\nSee [[Beta]].\n",
          },
        ],
        graph: { nodes: [{ id: "note_alpha", title: "Alpha" }], edges: [] },
        manifest: { profile: "public", renderer: "pinax-web" },
        searchIndex: [{ id: "note_alpha", title: "Alpha", text: "Alpha" }],
        assets: [{ path: "assets/logo.txt", content: "safe asset" }],
      },
      outDir,
    );

    for (const rel of [
      "index.html",
      "notes/alpha/index.html",
      "tags/public/index.html",
      "assets/logo.txt",
      "pinax-data/manifest.json",
      "pinax-data/graph.json",
      "pinax-data/search-index.json",
    ]) {
      await expect(stat(path.join(outDir, rel))).resolves.toBeTruthy();
      expect(result.files).toContain(rel);
    }
    const notePage = await readFile(path.join(outDir, "notes/alpha/index.html"), "utf8");
    expect(notePage).toContain("Alpha");
    expect(notePage).toContain('data-pinax-wikilink="Beta"');
    const manifest = JSON.parse(await readFile(path.join(outDir, "pinax-data/manifest.json"), "utf8"));
    expect(manifest).toEqual({ profile: "public", renderer: "pinax-web" });
  });
});
