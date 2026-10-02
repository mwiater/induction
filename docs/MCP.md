# MCP and application tools

Induction supports two tool systems. Configured MCP servers expose external
tools to the model through an MCP tool loop; local application tools are
managed by the host application and remain available when the invocation
supports them. They are separate from ordinary pipeline steps and are not
declared as YAML fields.

`InferMCPChat` provides multi-turn chat with configured MCP servers.
`InferMCPChatWithApproval` accepts an approval callback for tools that require
one. Read-only tools run automatically; side-effecting tools require the
application's approval policy.

Use `--nomcp` to disable configured MCP servers for one invocation. This does
not disable local application tools. Configure servers with `mcpServers` in
`induction.yaml`, and use the [CLI reference](CLI-REFERENCE.md) for the
`--nomcp` flag and related command options.

The [MCP pipeline examples](EXAMPLES-MCP-TOOLS.md) demonstrate discovery,
chained lookups, web research, JSON endpoint inspection, and multi-source
comparison.
