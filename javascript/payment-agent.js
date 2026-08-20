// The Invoice Payment Agent, end to end, with full safety architecture.
//
// Five steps: define the condition (invoice overdue, unpaid), evaluate
// it against a mock invoice database, present the reasoning and get
// human approval, execute the payment as a real USDC transfer on Fuji,
// and log the outcome, approved or not. Wrapped in four required safety
// pillars: pre-flight checks, a spending limit, an idempotency check,
// and an audit log entry for every decision.
//
// See kill_switch.sol in this folder for the fourth pillar, the on-chain
// kill switch, that one lives in the smart contract, not this script.

import "dotenv/config";
import { ethers } from "ethers";
import readline from "node:readline/promises";
import { stdin as input, stdout as output } from "node:process";

const rl = readline.createInterface({ input, output });

const FUJI_RPC = "https://api.avax-test.network/ext/bc/C/rpc";
const MAX_PAYMENT_USDC = Number(process.env.MAX_PAYMENT_USDC || 500); // spending limit, per transaction
const USDC_DECIMALS = 6; // USDC uses 6 decimals, not 18 like AVAX, this trips people up constantly
const SENT_PAYMENTS_LOG = new Set(); // idempotency, in memory for this demo

// Mock invoice database, standing in for a real one, same shape you'd
// get back from a real accounts-payable system or database query.
const invoices = [
  { id: "INV-042", supplier: "Supplier X", amountUsdc: 5, dueDate: "2026-08-14", paid: false, recipient: "0xA6dEb2d9570976730131fE7Dc7F9Dc268ECb48E5" },
  { id: "INV-043", supplier: "Supplier Y", amountUsdc: 120, dueDate: "2026-08-19", paid: false, recipient: "0xCd34EF56ab12cD34ef56aB12CD34eF56AB12cD34" },
  { id: "INV-044", supplier: "Supplier Z", amountUsdc: 30, dueDate: "2026-08-20", paid: true, recipient: "0xeF56ab12CD34Ef56ab12cD34Ef56AB12cD34ef56" },
];

function daysOverdue(dueDate) {
  const diffMs = new Date() - new Date(dueDate);
  return Math.floor(diffMs / (1000 * 60 * 60 * 24));
}

// Fail fast with a readable message if the wallet or token env vars are
// still the placeholders from .env.example, rather than crashing deep
// inside ethers with an opaque "invalid BytesLike value" later on.
function validateEnv() {
  const key = process.env.AGENT_PRIVATE_KEY;
  const usdc = process.env.FUJI_USDC_ADDRESS;

  if (!key || !/^0x[0-9a-fA-F]{64}$/.test(key)) {
    throw new Error(
      "AGENT_PRIVATE_KEY is missing or not a valid 0x-prefixed 64-hex-char key. " +
        "Generate a Fuji testnet-only key and set it in .env."
    );
  }
  if (!usdc || !ethers.isAddress(usdc)) {
    throw new Error(
      "FUJI_USDC_ADDRESS is missing or not a valid address. " +
        "Set it to the Fuji USDC contract in .env (e.g. 0x5425890298aed601595a70AB815c96711a31Bc65)."
    );
  }
}

// Step 1 and 2: define the condition, evaluate it against the mock database.
function findOverdueInvoices() {
  return invoices.filter((inv) => !inv.paid && daysOverdue(inv.dueDate) > 3);
}

// Safety pillar 1: pre-flight checks, run before any payment goes out,
// no exceptions. Spending limit (pillar 2) and the idempotency check are
// both enforced here too, alongside on-chain balance verification so we
// never broadcast a transfer that is guaranteed to revert.
async function preflightChecks(invoice, wallet, usdc) {
  const errors = [];

  // Recipient must be a well-formed, checksum-valid, non-zero address.
  let recipient;
  try {
    recipient = ethers.getAddress(invoice.recipient); // throws on bad checksum / malformed
    if (recipient === ethers.ZeroAddress) errors.push("recipient is the zero address");
  } catch {
    errors.push(`invalid recipient address: ${invoice.recipient}`);
  }

  if (invoice.amountUsdc > MAX_PAYMENT_USDC) {
    errors.push(`amount ${invoice.amountUsdc} exceeds spending limit of ${MAX_PAYMENT_USDC} USDC`);
  }
  if (SENT_PAYMENTS_LOG.has(invoice.id)) {
    errors.push("payment already sent for this invoice, idempotency check failed");
  }

  // On-chain balance checks: enough AVAX for gas, enough USDC to cover
  // the transfer. Catching this here turns an ugly estimateGas revert
  // into a clear, actionable message.
  try {
    const amount = ethers.parseUnits(invoice.amountUsdc.toString(), USDC_DECIMALS);
    const [avax, usdcBalance] = await Promise.all([
      wallet.provider.getBalance(wallet.address),
      usdc.balanceOf(wallet.address),
    ]);
    if (avax === 0n) {
      errors.push(`agent wallet ${wallet.address} has 0 AVAX for gas, fund it from a Fuji faucet`);
    }
    if (usdcBalance < amount) {
      errors.push(
        `agent wallet ${wallet.address} holds ${ethers.formatUnits(usdcBalance, USDC_DECIMALS)} USDC, ` +
          `needs ${invoice.amountUsdc} USDC, fund it with Fuji test USDC`
      );
    }
  } catch (err) {
    errors.push(`could not read on-chain balances: ${err.shortMessage || err.message}`);
  }

  return errors;
}

// Step 3: present reasoning, get human approval before anything executes.
async function confirmPayment(invoice, overdueDays) {
  console.log(`\nInvoice ${invoice.id} from ${invoice.supplier} is ${overdueDays} days overdue.`);
  console.log(`I intend to send ${invoice.amountUsdc} USDC to ${invoice.recipient}.`);
  const answer = await rl.question("Do you approve? (y/n): ");
  return answer.trim().toLowerCase() === "y";
}

// Step 4: on approval, actually execute the payment on Fuji.
async function sendPayment(invoice, usdc) {
  // USDC on Fuji is an ERC-20, transfer() takes the recipient and an
  // amount in the token's smallest unit, 6 decimals for USDC, not 18
  // like AVAX, this trips people up constantly.
  const amount = ethers.parseUnits(invoice.amountUsdc.toString(), USDC_DECIMALS);
  const tx = await usdc.transfer(ethers.getAddress(invoice.recipient), amount);
  await tx.wait();
  return tx.hash;
}

// Safety pillar 4 (the software half, the kill switch itself lives on
// chain, see kill_switch.sol): log every decision, approved or not.
function logDecision({ invoiceId, reasoning, approved, txHash }) {
  const entry = {
    timestamp: new Date().toISOString(),
    invoiceId,
    reasoning,
    approved,
    txHash: txHash ?? null,
  };
  console.log(JSON.stringify(entry));
  // production: append to a durable log store, not just stdout
}

async function main() {
  validateEnv();

  const provider = new ethers.JsonRpcProvider(FUJI_RPC);
  const wallet = new ethers.Wallet(process.env.AGENT_PRIVATE_KEY, provider);
  // balanceOf/decimals let us verify funds in pre-flight; transfer sends.
  const usdcAbi = [
    "function transfer(address to, uint256 amount) returns (bool)",
    "function balanceOf(address owner) view returns (uint256)",
    "function decimals() view returns (uint8)",
  ];
  const usdc = new ethers.Contract(process.env.FUJI_USDC_ADDRESS, usdcAbi, wallet);

  const overdue = findOverdueInvoices();

  for (const invoice of overdue) {
    const overdueDays = daysOverdue(invoice.dueDate);
    const reasoning = `${invoice.id} is ${overdueDays} days overdue, condition met (overdue > 3 days)`;

    const preflightErrors = await preflightChecks(invoice, wallet, usdc);
    if (preflightErrors.length > 0) {
      logDecision({ invoiceId: invoice.id, reasoning: `Pre-flight failed: ${preflightErrors.join(", ")}`, approved: false });
      continue;
    }

    const approved = await confirmPayment(invoice, overdueDays);
    if (!approved) {
      logDecision({ invoiceId: invoice.id, reasoning, approved: false });
      continue;
    }

    const txHash = await sendPayment(invoice, usdc);
    SENT_PAYMENTS_LOG.add(invoice.id);
    logDecision({ invoiceId: invoice.id, reasoning, approved: true, txHash });
    console.log(`Payment sent, transaction hash: ${txHash}`);
    console.log(`View it on Snowtrace: https://testnet.snowtrace.io/tx/${txHash}`);
  }

  rl.close();
}

main().catch((err) => {
  console.error("Payment agent error:", err.message);
  rl.close();
  process.exit(1);
});
