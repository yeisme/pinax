// A real SDK client catches transport/schema rejection that JSON-only tests miss.
// Run against a supplied build and installed SDK; never invoke an LLM or user vault.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';

const [binaryArg, sdkArg] = process.argv.slice(2);
if (!binaryArg || !sdkArg) throw new Error('Usage: node tools/testkit/mcpstrictclient.mjs <pinax-binary> <installed-sdk-directory>');
const binary = resolve(binaryArg), sdk = resolve(sdkArg);
const { Client } = await import(pathToFileURL(join(sdk, 'dist/esm/client/index.js')));
const { StdioClientTransport } = await import(pathToFileURL(join(sdk, 'dist/esm/client/stdio.js')));
const packageInfo = JSON.parse(await readFile(join(sdk, 'package.json'), 'utf8'));
const root = await mkdtemp(join(tmpdir(), 'pinax-strict-client-'));
let client;
try {
  const vault = join(root, 'vault');
  execFileSync(binary, ['init', vault, '--json'], { stdio: ['ignore', 'pipe', 'pipe'], timeout: 15000 });
  const errors = [];
  for (const collaboration of [false, true]) {
    const args = ['mcp', 'serve', '--vault', vault];
    if (collaboration) args.push('--collaboration', '--allow-note-body', '--allow-note-write');
    // PINAX_MCP_RUNTIME passthrough lets the same strict client validate the
    // official Go SDK candidate runtime without changing the spawn contract.
    const runtime = process.env.PINAX_MCP_RUNTIME;
    const transport = new StdioClientTransport({ command: binary, args, stderr: 'pipe', ...(runtime ? { env: { PINAX_MCP_RUNTIME: runtime } } : {}) });
    client = new Client({ name: 'pinax-strict-client-regression', version: '1.0.0' });
    client.onerror = () => errors.push('protocol_rejected');
    await client.connect(transport, { timeout: 5000 });
    const started = Date.now();
    const list = await client.listTools(undefined, { timeout: 5000 });
    assert(list.tools.some(t => t.name === 'pinax.search'));
    assert.equal(list.tools.some(t => t.name === 'pinax.interaction.apply'), collaboration);
    const resources = await client.listResources(undefined, { timeout: 5000 });
    assert(resources.resources.some(r => r.uri === 'pinax://manifest'));
    assert(!resources.resources.some(r => r.uri.startsWith('ui://')));
    assert(list.tools.every(t => !t._meta?.ui));
    await client.listResourceTemplates(undefined, { timeout: 5000 });
    await client.readResource({ uri: 'pinax://manifest' }, { timeout: 5000 });
    if (collaboration) {
      await assert.rejects(client.readResource({ uri: 'ui://pinax/collaboration-v1.html' }, { timeout: 5000 }));
      const preview = await client.callTool({ name: 'pinax.interaction.preview', arguments: { change: { action: 'create', title: 'SDK fixture', body: '# SDK fixture\n\nSynthetic content.' } } }, undefined, { timeout: 5000 });
      assert(!preview.isError);
      const proposal = preview.structuredContent.data;
      const applied = await client.callTool({ name: 'pinax.interaction.apply', arguments: { change: proposal.change, preview_digest: proposal.preview_digest, authorization: 'explicit_instruction' } }, undefined, { timeout: 5000 });
      assert(!applied.isError);
      assert.equal(applied.structuredContent.data.status, 'succeeded');
      const search = await client.callTool({ name: 'pinax.interaction.search', arguments: { query: 'SDK fixture' } }, undefined, { timeout: 5000 });
      assert(!search.isError);
    }
    assert.deepEqual(errors, []);
    console.log(`PASS sdk=${packageInfo.version} collaboration=${collaboration} tools=${list.tools.length} elapsed_ms=${Date.now() - started} protocol_errors=0 runtime=${runtime || 'legacy'}`);
    await client.close(); client = undefined;
  }
} finally {
  await client?.close();
  await rm(root, { recursive: true, force: true });
}
