# Session 6 Starter · Python

Autonomous payment agents and safety architecture, plus Docker support.
Same pattern as the JavaScript starter, Python idioms throughout.

## Setup

**With Docker** (no local Python install needed):

```bash
cp .env.example .env
# fill in ANTHROPIC_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
cd .. && docker compose run --rm python python payment_agent.py
```

**Without Docker:**

```bash
python3 -m venv venv
source venv/bin/activate   # or venv\Scripts\activate on Windows
pip install -r requirements.txt
cp .env.example .env
# fill in ANTHROPIC_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
```

## A critical safety note, read this before you run anything

`AGENT_PRIVATE_KEY` signs real transactions. Use a wallet you generated
specifically for this cohort, funded only with Fuji testnet AVAX and
USDC from a faucet, never a wallet that holds anything real. `.env` is
already in `.gitignore`, double-check before you push anyway.

## Files

| File | What it does |
|---|---|
| `model_provider.py` | Provider abstraction: `create_model_client()`, four providers, one shared async interface |
| `direct_rpc.py` | Method 1: raw JSON-RPC via `web3.py`, no external chain SDK needed |
| `chainkit_fetch.py` | Method 2: calls the Glacier REST API directly |
| `chainkit_mcp_agent.py` | ChainKit as MCP server, using the official `mcp` Python SDK |
| `advisor.py` | Session 4: the Smart Wallet Advisor, with a human-in-the-loop checkpoint and audit logging |
| `rag.py` | Session 5: retrieval-augmented generation, grounded answers with citations |
| `payment_agent.py` | Session 6: the invoice payment agent, evaluates a condition, gets human approval, sends real USDC on Fuji |
| `kill_switch.sol` | Session 6: the on-chain safety pillar, an `onlyOwner` Solidity kill switch |
| `normalize.py` | Shared wei-to-AVAX, hex-to-decimal, Unix-to-ISO8601 conversion |

## Running each one

```bash
python direct_rpc.py
python chainkit_fetch.py
python chainkit_mcp_agent.py
python advisor.py <wallet-address>
python rag.py "your question"           # needs Chroma running
python payment_agent.py                  # Session 6: the full payment agent
```

## How the payment agent's signing was verified

`web3.py` was used to build and sign a real transaction end to end
during development, not just syntax-checked, including confirming the
exact attribute name on the signed result (`raw_transaction`, not the
older `rawTransaction` some tutorials still reference). The derived
wallet address from a test private key matches exactly what the
JavaScript, Go, and Rust versions in this repo derive from that same
key, real cross-language proof the signing logic is correct.

## A note on the Solidity file

`kill_switch.sol` compiles cleanly with `solc` 0.8.35, no errors, no
warnings. It follows standard, well-established Solidity patterns and
was carefully hand-checked, but compiling clean isn't the same as
tested or audited, it hasn't been deployed or exercised against a live
network. Compile and test it yourself, on Remix or with Hardhat or
Foundry locally, before you deploy it.

## Model provider

`MODEL_PROVIDER` in `.env` picks the provider (`anthropic`, `openai`,
`gemini`, or `ollama`), defaulting to `anthropic`. This doesn't affect
`payment_agent.py`, which doesn't call a model at all, the condition
evaluation is plain code, not an LLM call, by design, you want that
logic fully deterministic and auditable when real money is involved.

## Submission

1. Test everything yourself, confirm your payment agent evaluates the condition correctly, asks for approval, and sends a real transaction on approval.
2. Screenshot the working test, including the approval prompt and the resulting Snowtrace transaction.
3. Open your PR, screenshot that too.
4. Post on X with both screenshots, tag **@code_mwangi** and **@AvaxAfrica**.
5. Copy your post link, submit it on the quest page once it's live.

Post in the Week 3 WhatsApp group for anything you get stuck on.
