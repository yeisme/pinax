import matter from "gray-matter";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import rehypeStringify from "rehype-stringify";
import remarkGfm from "remark-gfm";
import remarkParse from "remark-parse";
import remarkRehype from "remark-rehype";
import { unified } from "unified";
import { visit } from "unist-util-visit";

export interface RenderMarkdownResult {
  html: string;
  metadata: Record<string, unknown>;
}

export async function renderMarkdown(source: string): Promise<RenderMarkdownResult> {
  const parsed = matter(source);
  const body = stripExecutableDirectives(parsed.content);
  const file = await unified()
    .use(remarkParse)
    .use(remarkGfm)
    .use(pinaxRemarkPlaceholders)
    .use(remarkRehype)
    .use(rehypeSanitize, pinaxSanitizeSchema)
    .use(rehypeStringify)
    .process(body);

  return {
    html: String(file),
    metadata: JSON.parse(JSON.stringify(parsed.data ?? {})) as Record<string, unknown>,
  };
}

function stripExecutableDirectives(source: string): string {
  return source
    .split("\n")
    .filter((line) => !/^\s*(import|export)\s+/.test(line))
    .join("\n");
}

function pinaxRemarkPlaceholders() {
  return (tree: any) => {
    visit(tree, (node: any, index: number | undefined, parent: any) => {
      if (!parent || typeof index !== "number") {
        return;
      }
      if (node.type === "text") {
        const replacement = splitPinaxInlineTokens(node.value);
        if (replacement) {
          parent.children.splice(index, 1, ...replacement);
        }
        return;
      }
      if (node.type === "code") {
        const placeholder = codePlaceholderType(node.lang ?? "");
        if (placeholder) {
          parent.children.splice(index, 1, placeholderNode(placeholder));
        }
        return;
      }
      if ((node.type === "link" || node.type === "image") && isNetworkURL(node.url)) {
        parent.children.splice(index, 1, placeholderNode("network-blocked"));
      }
    });
  };
}

function splitPinaxInlineTokens(value: string): any[] | undefined {
  const pattern = /(!)?\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/g;
  const nodes: any[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(value)) !== null) {
    if (match.index > lastIndex) {
      nodes.push({ type: "text", value: value.slice(lastIndex, match.index) });
    }
    const isAttachment = match[1] === "!";
    const target = match[2]?.trim() ?? "";
    const label = (match[3] ?? target).trim();
    nodes.push(isAttachment ? attachmentNode(target) : wikilinkNode(target, label));
    lastIndex = pattern.lastIndex;
  }

  if (nodes.length === 0) {
    return undefined;
  }
  if (lastIndex < value.length) {
    nodes.push({ type: "text", value: value.slice(lastIndex) });
  }
  return nodes;
}

function wikilinkNode(target: string, label: string): any {
  return {
    type: "link",
    url: `#${slug(target)}`,
    data: { hProperties: { dataPinaxWikilink: target } },
    children: [{ type: "text", value: label || target }],
  };
}

function attachmentNode(target: string): any {
  return {
    type: "emphasis",
    data: {
      hName: "span",
      hProperties: { className: ["pinax-placeholder"], dataPinaxAttachment: target },
    },
    children: [{ type: "text", value: `Attachment: ${target}` }],
  };
}

function placeholderNode(kind: string): any {
  return {
    type: "paragraph",
    data: {
      hName: "div",
      hProperties: { className: ["pinax-placeholder"], dataPinaxPlaceholder: kind },
    },
    children: [{ type: "text", value: placeholderLabel(kind) }],
  };
}

function codePlaceholderType(lang: string): string {
  const normalized = lang.toLowerCase().trim();
  if (normalized.startsWith("pinax-managed")) {
    return "managed-block";
  }
  if (normalized === "dataview") {
    return "dataview";
  }
  if (normalized === "pinax-database" || normalized === "database") {
    return "database";
  }
  return "";
}

function placeholderLabel(kind: string): string {
  switch (kind) {
    case "managed-block":
      return "Managed block placeholder";
    case "dataview":
      return "Dataview result placeholder";
    case "database":
      return "Database result placeholder";
    case "network-blocked":
      return "Network resource blocked";
    default:
      return "Pinax placeholder";
  }
}

function isNetworkURL(value: string): boolean {
  return /^https?:\/\//i.test(value.trim());
}

function slug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

const pinaxSanitizeSchema = {
  ...defaultSchema,
  attributes: {
    ...defaultSchema.attributes,
    a: [...(defaultSchema.attributes?.a ?? []), "dataPinaxWikilink"],
    div: [...(defaultSchema.attributes?.div ?? []), "className", "dataPinaxPlaceholder"],
    span: [...(defaultSchema.attributes?.span ?? []), "className", "dataPinaxAttachment"],
  },
};
