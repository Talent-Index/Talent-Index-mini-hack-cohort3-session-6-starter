//! ChainKit as an MCP server, wired into an agent.
//!
//! Before running this, start the ChainKit MCP server in another
//! terminal:
//!
//! ```text
//! npx -y @avalanche-sdk/chainkit start --transport sse --port 2718
//! ```
//!
//! It will print the local URL it is running on, e.g.
//! `http://localhost:2718/sse`. Put that URL in `CHAINKIT_MCP_URL` in
//! your `.env`. The CLI's only two transports are `stdio` and `sse`
//! (verified against the installed package, there is no `mcp-server`
//! subcommand and no streamable-http mode).
//!
//! CAUTION: this file still connects with `StreamableHttpClientTransport`,
//! not a match for the server's `sse` transport. `rmcp` 3.1.2, the
//! version pinned in `Cargo.lock`, does not expose a client-side
//! transport for the legacy two-endpoint SSE protocol the CLI actually
//! serves (only an internal SSE detail used by the streamable-http
//! client, not a standalone SSE client transport type), so there is no
//! drop-in fix within this crate version. A newer `rmcp` release, or a
//! hand-rolled `reqwest`-based SSE client, would be needed to actually
//! connect this file to that server.
//!
//! NOTE: as of @avalanche-sdk/chainkit@0.3.13, the latest version on npm
//! at the time this was checked, the bundled server fails to start at
//! all, for both transports, with "Schema method literal must be a
//! string" thrown while constructing its internal MCP server. That's a
//! bug in the published package, not in this file, and it has nothing to
//! do with which transport or URL you use. This demo path won't work
//! tonight regardless of language or the caveat above, use
//! chainkit_fetch.rs for the same on-chain data instead.
//!
//! This agent then asks a plain-English question, the model decides to
//! call a ChainKit tool, and the tool call is forwarded straight to the
//! running MCP server, no manual SDK calls for the actual data lookup
//! in this file at all.
//!
//! This file talks to Anthropic directly rather than through
//! [`mini_hack_session6::modelprovider`], because tool-call round trips
//! need the model's own native response shape threaded through the
//! loop, and `modelprovider`'s `Message` type only covers plain text
//! turns today. See that module's doc comment for why.

use rmcp::model::CallToolRequestParams;
use rmcp::transport::StreamableHttpClientTransport;
use rmcp::ServiceExt;
use serde_json::{json, Map, Value};
use std::env;
use std::io::{self, Write};

const SYSTEM_PROMPT: &str = "You are Mini Hack Assistant. Use tools when they genuinely help; otherwise answer directly.";

fn mcp_tools_to_anthropic(tools: &[rmcp::model::Tool]) -> Vec<Value> {
    tools
        .iter()
        .map(|t| {
            json!({
                "name": t.name,
                "description": t.description.clone().unwrap_or_default(),
                "input_schema": t.input_schema,
            })
        })
        .collect()
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let _ = dotenvy::dotenv();

    let api_key = env::var("ANTHROPIC_API_KEY").map_err(|_| "ANTHROPIC_API_KEY is not set.")?;
    let mcp_url =
        env::var("CHAINKIT_MCP_URL").map_err(|_| "Set CHAINKIT_MCP_URL in your .env first, from the running server's output.")?;
    let model = env::var("ANTHROPIC_MODEL").unwrap_or_else(|_| "claude-sonnet-4-6".to_string());

    let http = reqwest::Client::new();

    let transport = StreamableHttpClientTransport::from_uri(mcp_url);
    let mcp_client = ().serve(transport).await?;

    let tools_result = mcp_client.list_tools(None).await?;
    let tools = mcp_tools_to_anthropic(&tools_result.tools);

    println!("Mini Hack on-chain agent using anthropic, connected to ChainKit MCP. Type 'exit' to quit.\n");

    let mut messages: Vec<Value> = Vec::new();

    loop {
        print!("You: ");
        io::stdout().flush()?;
        let mut user_input = String::new();
        io::stdin().read_line(&mut user_input)?;
        let user_input = user_input.trim();
        if user_input.eq_ignore_ascii_case("exit") {
            break;
        }

        messages.push(json!({ "role": "user", "content": user_input }));

        let mut resp = call_anthropic(&http, &api_key, &model, &messages, &tools).await?;

        while resp["stop_reason"].as_str() == Some("tool_use") {
            messages.push(json!({ "role": "assistant", "content": resp["content"] }));

            let mut tool_results = Vec::new();
            if let Some(blocks) = resp["content"].as_array() {
                for block in blocks {
                    if block["type"].as_str() != Some("tool_use") {
                        continue;
                    }
                    let tool_id = block["id"].as_str().unwrap_or("").to_string();
                    let tool_name = block["name"].as_str().unwrap_or("").to_string();
                    let input = block["input"].as_object().cloned().unwrap_or_else(Map::new);

                    let params = CallToolRequestParams::new(tool_name).with_arguments(input);
                    match mcp_client.call_tool(params).await {
                        Ok(result) => {
                            let content_json = serde_json::to_string(&result.content).unwrap_or_default();
                            tool_results.push(json!({
                                "type": "tool_result",
                                "tool_use_id": tool_id,
                                "content": content_json,
                            }));
                        }
                        Err(e) => {
                            tool_results.push(json!({
                                "type": "tool_result",
                                "tool_use_id": tool_id,
                                "content": format!("Error: {e}"),
                                "is_error": true,
                            }));
                        }
                    }
                }
            }

            messages.push(json!({ "role": "user", "content": tool_results }));
            resp = call_anthropic(&http, &api_key, &model, &messages, &tools).await?;
        }

        let final_text = resp["content"]
            .as_array()
            .and_then(|blocks| blocks.iter().find(|b| b["type"] == "text"))
            .and_then(|b| b["text"].as_str())
            .unwrap_or("");
        println!("\nAssistant: {final_text}\n");

        messages.push(json!({ "role": "assistant", "content": resp["content"] }));
    }

    Ok(())
}

async fn call_anthropic(
    http: &reqwest::Client,
    api_key: &str,
    model: &str,
    messages: &[Value],
    tools: &[Value],
) -> Result<Value, Box<dyn std::error::Error>> {
    let body = json!({
        "model": model,
        "max_tokens": 1024,
        "system": SYSTEM_PROMPT,
        "messages": messages,
        "tools": tools,
    });

    let resp = http
        .post("https://api.anthropic.com/v1/messages")
        .header("x-api-key", api_key)
        .header("anthropic-version", "2023-06-01")
        .header("content-type", "application/json")
        .json(&body)
        .send()
        .await?;

    Ok(resp.json().await?)
}
