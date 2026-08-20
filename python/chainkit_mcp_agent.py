"""
ChainKit as an MCP server, wired into an agent.

Before running this, start the ChainKit MCP server in another terminal:
    npx -y @avalanche-sdk/chainkit start --transport sse --port 2718
It will print the local URL it is running on, e.g. http://localhost:2718/sse
Put that URL in CHAINKIT_MCP_URL in your .env. The CLI's only two
transports are stdio and sse (verified against the installed package,
there is no `mcp-server` subcommand and no streamable-http mode), so
this file uses mcp.client.sse.sse_client, not streamable_http_client.

NOTE: as of @avalanche-sdk/chainkit@0.3.13, the latest version on npm at
the time this was checked, the bundled server fails to start at all, for
both transports, with "Schema method literal must be a string" thrown
while constructing its internal MCP server. That's a bug in the
published package, not in this file, and it has nothing to do with which
transport or URL you use. If it's still broken tonight, this demo path
won't work regardless of language, use chainkit_fetch.py for the same
on-chain data instead.

This agent then asks a plain-English question, the model decides to call
a ChainKit tool, and the tool call is forwarded straight to the running
MCP server, no manual SDK calls for the actual data lookup in this file
at all.
"""

import asyncio
import os

from dotenv import load_dotenv
from mcp import ClientSession
from mcp.client.sse import sse_client

from model_provider import create_model_client

load_dotenv()

SYSTEM_PROMPT = "You are Mini Hack Assistant. Use tools when they genuinely help; otherwise answer directly."


def _mcp_tools_to_anthropic_format(mcp_tools) -> list:
    # MCP's Tool type uses inputSchema (camelCase). Anthropic's tools
    # parameter expects input_schema (snake_case). This is an easy
    # mismatch to miss, converting explicitly here rather than assuming
    # the shapes line up.
    return [
        {"name": t.name, "description": t.description or "", "input_schema": t.inputSchema}
        for t in mcp_tools
    ]


async def main():
    url = os.environ.get("CHAINKIT_MCP_URL")
    if not url:
        raise ValueError("Set CHAINKIT_MCP_URL in your .env first, from the running server's output.")

    client = create_model_client()
    messages = []

    async with sse_client(url) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            tools_result = await session.list_tools()
            tools = _mcp_tools_to_anthropic_format(tools_result.tools)

            print(f"Mini Hack on-chain agent using {client.provider}, connected to ChainKit MCP. Type 'exit' to quit.\n")

            while True:
                user_input = input("You: ")
                if user_input.strip().lower() == "exit":
                    break

                messages.append({"role": "user", "content": user_input})

                response = await client.generate_text(SYSTEM_PROMPT, messages, tools=tools)

                while response.stop_reason == "tool_use":
                    messages.append({"role": "assistant", "content": response.raw.content})

                    tool_results = []
                    for call in response.tool_calls:
                        try:
                            result = await session.call_tool(call.name, call.input)
                            tool_results.append({
                                "type": "tool_result",
                                "tool_use_id": call.id,
                                "content": str(result.content),
                            })
                        except Exception as err:
                            tool_results.append({
                                "type": "tool_result",
                                "tool_use_id": call.id,
                                "content": f"Error: {err}",
                                "is_error": True,
                            })

                    messages.append({"role": "user", "content": tool_results})
                    response = await client.generate_text(SYSTEM_PROMPT, messages, tools=tools)

                print(f"\nAssistant: {response.text}\n")
                messages.append({"role": "assistant", "content": response.raw.content})


if __name__ == "__main__":
    asyncio.run(main())
