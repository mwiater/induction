# MCP and application tools

Induction supports two tool systems. Configured MCP servers expose external
tools to the model through an MCP tool loop; local application tools are
managed by the host application and remain available when the invocation
supports them. Pipeline steps do not use MCP by default; a step opts in with
`usemcp: true`.

`InferMCPChat` provides multi-turn chat with configured MCP servers.
`InferMCPChatWithApproval` accepts an approval callback for tools that require
one. Read-only tools run automatically; side-effecting tools require the
application's approval policy.

Configure servers with `mcpServers` in `induction.yaml`. MCP pipeline examples
set `usemcp: true` on every step that needs the configured servers. Direct
inference uses `--usemcp` to opt into MCP; local application tools remain
available by default. See the [CLI reference](CLI-REFERENCE.md).

The [MCP pipeline examples](EXAMPLES-MCP-TOOLS.md) demonstrate discovery,
chained lookups, web research, JSON endpoint inspection, and multi-source
comparison.
