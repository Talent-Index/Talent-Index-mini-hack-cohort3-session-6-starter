# Mini Hack · Cohort 3, Session 6 Starter

**Building Agentic Solutions on Avalanche** · Team1 Kenya

Autonomous payment agents and safety architecture, in four languages,
plus Docker support. Builds directly on the Session 5 starter, same
model-provider layer, same on-chain data methods, same RAG pipeline,
now extended with an agent that evaluates a real condition and actually
moves USDC on Fuji, gated behind four required safety pillars.

## What's new this session

Every language folder gets two new files:

**`payment-agent`** (`payment-agent.js`, `payment_agent.py`,
`payment-agent/main.go`, `src/bin/payment_agent.rs`), which does five
things:

1. Defines a condition against a mock invoice database, invoice unpaid
   and overdue by more than 3 days
2. Evaluates that condition, finding every invoice that matches
3. Presents its reasoning in plain English and waits for explicit human
   approval before doing anything
4. On approval, executes a real USDC transfer on Fuji, signed with a
   testnet-only private key
5. Logs the outcome, approved, rejected, or blocked by a pre-flight
   check, every single time

**`kill_switch.sol`**, identical across all four folders since it's
chain code, not language-specific. An `onlyOwner` Solidity contract that
pauses every agent-initiated payment on a single transaction. This is
the fourth safety pillar, and the only one that lives on chain instead
of in application code.

Everything else, the model provider layer, direct RPC, ChainKit,
the RAG pipeline, the Smart Wallet Advisor, is carried forward unchanged
from Session 5.

## The four safety pillars, and where each one lives

| Pillar | Where |
|---|---|
| Pre-flight checks (valid recipient, spending limit, idempotency) | `preflightChecks` / `preflight_checks`, runs before every payment |
| Spending limit | `MAX_PAYMENT_USDC` in `.env`, enforced inside pre-flight checks |
| Kill switch | `kill_switch.sol`, on chain, independent of your application code |
| Audit log | `logDecision` / `log_decision`, one structured entry per decision, approved or not |

All four, every time, no exceptions. That's the whole point of the
session.

## Running with Docker

Same pattern as Session 5. From the repo root:

```bash
docker compose run --rm javascript npm run payment-agent
docker compose run --rm python python payment_agent.py
docker compose run --rm golang ./payment-agent
docker compose run --rm rust ./payment_agent
```

No Chroma needed for this one, so you can skip `docker compose up -d
chroma` if you're only running the payment agent. Fill in
`AGENT_PRIVATE_KEY`, `FUJI_USDC_ADDRESS`, and `MAX_PAYMENT_USDC` in your
`.env` first, same as running locally.

## Running without Docker

Install that language's dependencies, copy `.env.example` to `.env`,
fill in a **Fuji testnet-only** private key, and run the script.

## A critical safety note, read this before you run anything

`AGENT_PRIVATE_KEY` signs real transactions. Use a wallet you generated
specifically for this cohort, funded only with Fuji testnet AVAX and
USDC from a faucet, never a wallet that holds anything real. Never
commit `.env` with a real value filled in, it's already in every
language's `.gitignore`, but double-check before you push.

## How the transaction signing was actually verified

This is the part of Session 6 with the least room for getting something
subtly wrong, since it moves real money. Before any of this shipped:

- **JavaScript**: `ethers.js` installed fresh, a real `Wallet` and
  `Contract` constructed and exercised directly, not just syntax-checked
- **Python**: `web3.py` used to build and sign a real transaction end to
  end, confirming the exact attribute names on the signed result
- **Go**: there is no lightweight, fully-resolvable all-in-one Ethereum
  SDK for Go in every network environment, the full `go-ethereum` client
  pulls in a very large dependency tree. This code uses two focused
  subpackages instead, `crypto` for signing and `rlp` for transaction
  encoding, both compiled and run against a real test key
- **Rust**: `ethers-rs`, the standard Rust Ethereum crate, compiled and
  exercised directly

**All four independently derive the identical wallet address from the
same test private key.** That's real cross-language proof the signing
logic is correct in each one, not four separate guesses. The Go and
Rust versions include this exact check as a real, runnable test in the
file itself, see `main_test.go` in the Go folder and the `#[cfg(test)]`
block at the bottom of `payment_agent.rs` in Rust. The JavaScript and
Python checks were run standalone during development rather than
shipped as an in-repo test, since neither has a test runner already
configured in this starter.

## A note on the Solidity file specifically

`kill_switch.sol` compiles cleanly with `solc` 0.8.35
(`solc --bin --abi kill_switch.sol`), no errors, no warnings, valid
bytecode and ABI out the other end. It follows standard,
well-established patterns (an `onlyOwner` modifier, a `paused` boolean
guard), the exact shape used across countless production contracts.

Compiling clean isn't the same as tested or audited: it hasn't been
deployed, exercised against a live network, or reviewed by anyone but
the person who wrote it. Compile and test it yourself, on Remix or with
Hardhat or Foundry locally, and get a second set of eyes on it, before
you deploy it. This is stated plainly in the file's own comments too.

## Picking a language

Same guidance as before, pick the one you're building your Week 3
deliverable in:

| Language | Folder |
|---|---|
| JavaScript | [`javascript/`](./javascript) |
| Python | [`python/`](./python) |
| Go | [`golang/`](./golang) |
| Rust | [`rust/`](./rust) |

## Submission

Test everything yourself, screenshot the working test and your PR, post
on X tagging **@code_mwangi** and **@AvaxAfrica**, then submit that link
on the quest page. Full steps are in each language folder's README.
