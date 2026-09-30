# MCP and application tools

`InferMCPChat` provides multi-turn chat with configured MCP servers.
`InferMCPChatWithApproval` accepts an approval callback for tools that require
one. Read-only tools run automatically.

Use `--nomcp` to disable configured MCP servers for one command. Local
application tools remain available when supported by the invocation.
