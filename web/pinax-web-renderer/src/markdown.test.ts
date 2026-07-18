import { describe, expect, it } from "vitest";

import { renderMarkdown } from "./markdown";

describe("renderMarkdown", () => {
  it("renders GFM, frontmatter, wikilinks, attachments, and safe placeholders", async () => {
    const result = await renderMarkdown(`---
title: Public Note
tags:
  - publish
---

# Public Note

- [x] GFM task
- [ ] Draft task

See [[Research/Alpha|Alpha note]] and ![[diagram.png]].

\`\`\`pinax-managed name=daily-review
private managed body must not render
\`\`\`

\`\`\`dataview
TABLE file.name FROM #publish
\`\`\`

\`\`\`pinax-database
view: published
\`\`\`
`);

    expect(result.metadata).toEqual({ title: "Public Note", tags: ["publish"] });
    expect(result.html).toContain("Public Note");
    expect(result.html).toContain("task-list-item");
    expect(result.html).toContain('data-pinax-wikilink="Research/Alpha"');
    expect(result.html).toContain("Alpha note");
    expect(result.html).toContain('data-pinax-attachment="diagram.png"');
    expect(result.html).toContain('data-pinax-placeholder="managed-block"');
    expect(result.html).toContain('data-pinax-placeholder="dataview"');
    expect(result.html).toContain('data-pinax-placeholder="database"');
    expect(result.html).not.toContain("private managed body must not render");
  });

  it("strips executable Markdown and HTML surfaces", async () => {
    const result = await renderMarkdown(`import Widget from "https://evil.example/widget";

<script>window.secret = "RAW_SCRIPT"</script>
<iframe src="https://evil.example/embed"></iframe>

[network](https://evil.example/path)
![pixel](https://evil.example/pixel.png)

# Safe
`);

    expect(result.html).toContain("Safe");
    for (const forbidden of ["<script", "RAW_SCRIPT", "<iframe", "https://evil.example", "import Widget", "src="]) {
      expect(result.html).not.toContain(forbidden);
    }
  });
});
