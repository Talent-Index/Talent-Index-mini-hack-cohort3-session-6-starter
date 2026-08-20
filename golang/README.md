# Session 6 Starter · Go

Autonomous payment agents and safety architecture, plus Docker support.
Compiled, statically typed, no runtime dependency install needed once
you've built it.

## Setup

**With Docker** (no local Go install needed):

```bash
cp .env.example .env
# fill in ANTHROPIC_API_KEY, OPENAI_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
cd .. && docker compose run --rm golang ./payment-agent
```

**Without Docker:**

```bash
go mod download
cp .env.example .env
# fill in ANTHROPIC_API_KEY, OPENAI_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
```

Requires Go 1.23 or newer if building outside Docker.

## A critical safety note, read this before you run anything

`AGENT_PRIVATE_KEY` signs real transactions. Use a wallet you generated
specifically for this cohort, funded only with Fuji testnet AVAX and
USDC from a faucet, never a wallet that holds anything real. `.env` is
already in `.gitignore`, double-check before you push anyway.

## Layout

| Path | What it is |
|---|---|
| `modelprovider/` | Provider abstraction package: `NewModelClient()`, four providers, one shared interface |
| `normalize/` | Shared wei-to-AVAX, hex-to-decimal, Unix-to-ISO8601 conversion package |
| `direct-rpc/` | Method 1: raw JSON-RPC over plain HTTP, no SDK at all |
| `chainkit-fetch/` | Method 2: calls the Glacier REST API directly |
| `chainkit-mcp-agent/` | ChainKit as MCP server, using `github.com/mark3labs/mcp-go` |
| `advisor/` | Session 4: the Smart Wallet Advisor, with a human-in-the-loop checkpoint and audit logging |
| `rag/` | Session 5: retrieval-augmented generation, grounded answers with citations |
| `payment-agent/` | Session 6: the invoice payment agent, evaluates a condition, gets human approval, sends real USDC on Fuji, includes `main_test.go` |
| `kill_switch.sol` | Session 6: the on-chain safety pillar, an `onlyOwner` Solidity kill switch |

## Running each one

```bash
go run ./direct-rpc
go run ./chainkit-fetch
go run ./chainkit-mcp-agent
go run ./advisor <wallet-address>
go run ./rag "your question"            # needs Chroma running
go run ./payment-agent                   # Session 6: the full payment agent
```

## How the payment agent's signing was actually built and verified

There is no single, all-in-one Ethereum SDK for Go that resolves
cleanly in every network environment the way `ethers.js` does for
JavaScript, the full `go-ethereum` client pulls in a very large
dependency tree, including packages that need hosts blocked in some
sandboxed environments. `payment-agent/main.go` uses two small, focused
subpackages instead:

- `github.com/ethereum/go-ethereum/crypto` for Keccak256 hashing and
  ECDSA signing
- `github.com/ethereum/go-ethereum/rlp` for transaction encoding

The transaction itself is then submitted over plain JSON-RPC with
`eth_sendRawTransaction`, the same pattern `direct-rpc` already uses for
reads.

Run the real, included test suite yourself to see this verified:

```bash
go test ./payment-agent/... -v
```

One of those tests, `TestKeyDerivationMatchesOtherLanguages`, derives a
wallet address from a fixed test private key and asserts it matches
exactly what the JavaScript, Python, and Rust versions in this repo
derive from that same key. That's real, runnable, cross-language proof
the signing logic here is correct, not a claim you have to take on
faith.

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
`payment-agent`, which doesn't call a model at all, the condition
evaluation is plain code, not an LLM call, by design, you want that
logic fully deterministic and auditable when real money is involved.
Note also that the Gemini client in `modelprovider.go` talks to the
REST API directly rather than through Google's official Go SDK, that
SDK couldn't be verified to compile in the environment this was built
in, plain REST avoids the dependency entirely.

## Submission

1. Test everything yourself, confirm your payment agent evaluates the condition correctly, asks for approval, and sends a real transaction on approval.
2. Screenshot the working test, including the approval prompt and the resulting Snowtrace transaction.
3. Open your PR, screenshot that too.
4. Post on X with both screenshots, tag **@code_mwangi** and **@AvaxAfrica**.
5. Copy your post link, submit it on the quest page once it's live.

Post in the Week 3 WhatsApp group for anything you get stuck on.
