import { z } from 'zod';

// The local CLIs Settings explains (a code host's). Type only: the bridge trusts the wire shape, like
// every sibling method.
const cliToolSchema = z.object({
  kind: z.literal('host'),
  provider: z.string(),
  label: z.string(),
  cli: z.string(),
  setup: z.object({
    docsUrl: z.string(),
    steps: z.array(z.object({ purpose: z.string(), command: z.string() })),
  }),
  status: z.object({
    kind: z.string(),
    path: z.string(),
    version: z.string(),
    account: z.string(),
    site: z.string(),
    reason: z.string(),
    probed: z.array(z.string()),
  }),
});
const cliToolsResultSchema = z.object({ tools: z.array(cliToolSchema) });
export type CliTool = z.infer<typeof cliToolSchema>;
export type CliToolsResult = z.infer<typeof cliToolsResultSchema>;
