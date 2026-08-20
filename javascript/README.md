# Session 6 Starter · JavaScript

Autonomous payment agents and safety architecture, plus Docker support.
Builds on Session 5's RAG agent and the model-provider pattern from
Sessions 1 and 2.

## Setup

**Without Docker:**

```bash
npm install
cp .env.example .env
# then fill in the values below
```

**With Docker** (no local Node install needed):

```bash
cp .env.example .env
# then fill in the values below
cd .. && docker compose run --rm javascript npm run payment-agent
```

### What to put in `.env` for the payment agent

| Variable | What to set it to |
|---|---|
| `AGENT_PRIVATE_KEY` | A Fuji **testnet-only** private key (`0x` + 64 hex). Generate one: `node -e "console.log(require('ethers').Wallet.createRandom().privateKey)"` |
| `FUJI_USDC_ADDRESS` | Fuji USDC contract: `0x5425890298aed601595a70AB815c96711a31Bc65` (Circle's official Fuji USDC — verify on [testnet Snowtrace](https://testnet.snowtrace.io/)) |
| `MAX_PAYMENT_USDC` | Per-transaction spending limit, defaults to `500` |

`payment-agent.js` calls `validateEnv()` on startup, so if either value is
still a placeholder you get a clear error instead of an opaque crash deep
inside `ethers`.

### Fund the agent wallet before you run

The agent signs and broadcasts a **real** transfer, so the wallet behind
`AGENT_PRIVATE_KEY` needs, on Fuji:

- **Test AVAX** for gas — [faucet.avax.network](https://faucet.avax.network/)
- **Test USDC** (at least the invoice amount) — [faucet.circle.com](https://faucet.circle.com) → select **Avalanche Fuji**. This dispenses exactly the `0x5425…` USDC above. The generic Core/Avalanche faucet may hand you a *different* test-USDC token that won't match `FUJI_USDC_ADDRESS`.

Print the wallet's address to know where to send funds:

```bash
node -e "require('dotenv/config'); console.log(new (require('ethers').Wallet)(process.env.AGENT_PRIVATE_KEY).address)"
```

The demo invoice (`INV-042`) is **5 USDC**, small enough to cover from a
single faucet drip. A built-in pre-flight balance check refuses to
broadcast (and logs why) if the wallet is underfunded, so it never wastes
gas on a transfer that would revert.

## A critical safety note, read this before you run anything

`AGENT_PRIVATE_KEY` signs real transactions. Use a wallet you generated
specifically for this cohort, funded only with Fuji testnet AVAX and
USDC from a faucet, never a wallet that holds anything real. `.env` is
already in `.gitignore`, double-check before you push anyway.

## Files

| File | What it does |
|---|---|
| `model-provider.js` | Same provider abstraction from Session 2, carried forward unchanged |
| `direct-rpc.js` | Method 1: raw RPC via `ethers.js`, `getBalance`/`getBlock`/`getTransactionCount` |
| `chainkit-fetch.js` | Method 2: structured wallet history via the real `@avalanche-sdk/chainkit` SDK |
| `chainkit-mcp-agent.js` | ChainKit running as an MCP server, wired into a tool-calling agent |
| `advisor.js` | Session 4: the Smart Wallet Advisor, with a human-in-the-loop checkpoint and audit logging |
| `rag.js` | Session 5: retrieval-augmented generation, grounded answers with citations |
| `payment-agent.js` | Session 6: the invoice payment agent, evaluates a condition, gets human approval, sends real USDC on Fuji |
| `kill_switch.sol` | Session 6: the on-chain safety pillar, an `onlyOwner` Solidity kill switch |
| `normalize.js` | Shared wei-to-AVAX, hex-to-decimal, Unix-to-ISO8601 conversion, used by all data methods |

## Running each one

```bash
npm run direct-rpc
npm run fetch-transactions
npm run mcp-agent
npm run advisor -- <wallet-address>
npm run rag -- "your question"          # needs Chroma running
npm run payment-agent                    # Session 6: the full payment agent
```

## Payment agent in action

1. **Show the safety-first refusal.** With the wallet unfunded (or an
   amount over the limit), run `npm run payment-agent`. Pre-flight catches
   it and logs a JSON decision with `approved: false` and the reason — it
   never even asks for approval. This is the point: the agent won't
   broadcast a payment it can't or shouldn't make.
2. **Fund the wallet** with Fuji AVAX + USDC (see *Fund the agent wallet*
   above), then run it again.
3. **Show the human-in-the-loop checkpoint.** The agent prints its
   reasoning ("Invoice INV-042 … is N days overdue") and asks
   `Do you approve? (y/n)`. Answer `n` once to show it logs a declined
   decision and sends nothing.
4. **Approve it.** Run once more, answer `y`. The agent signs and
   broadcasts a real USDC transfer, waits for confirmation, and prints the
   transaction hash plus a Snowtrace link.
5. **Open the Snowtrace link** to show the on-chain transfer, and point to
   `kill_switch.sol` as the fourth pillar (on-chain kill switch) that would
   back all of this in production.

The four safety pillars to call out while demoing: **pre-flight checks**
(incl. on-chain balance verification), the **spending limit**
(`MAX_PAYMENT_USDC`), the **idempotency check** (no double-spend per
invoice), and the **audit log** (every decision emitted as JSON).


## Model provider

`MODEL_PROVIDER` in `.env` picks the provider (`anthropic`, `openai`,
`gemini`, or `ollama`), defaulting to `anthropic`. This doesn't affect
`payment-agent.js`, which doesn't call a model at all, the condition
evaluation is plain code, not an LLM call, by design, you want that
logic fully deterministic and auditable when real money is involved.

## Submission

1. Test everything yourself, confirm your payment agent evaluates the condition correctly, asks for approval, and sends a real transaction on approval.
2. Screenshot the working test, including the approval prompt and the resulting Snowtrace transaction.
3. Open your PR, screenshot that too.
4. Post on X with both screenshots, tag **@code_mwangi** and **@AvaxAfrica**.
5. Copy your post link, submit it on the quest page once it's live.

Post in the Week 3 WhatsApp group for anything you get stuck on.
