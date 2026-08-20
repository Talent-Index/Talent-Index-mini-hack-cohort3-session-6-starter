// Method 2: The ChainKit SDK
//
// Avalanche's official on-chain data SDK. Structured, paginated results,
// no manual RPC parsing. Needs a free Glacier API key from avacloud.io.
//
// The SDK's client class is `Avalanche` (not `AvalancheSDK`), and native
// transaction history lives at `sdk.data.evm.address.transactions.listNative`,
// verified directly against @avalanche-sdk/chainkit@0.3.13, the latest
// version published to npm at the time this was fixed.

import "dotenv/config";
import { Avalanche } from "@avalanche-sdk/chainkit";
import { normalizeMany } from "./normalize.js";

async function main() {
  const walletAddress = process.env.WALLET_ADDRESS;
  const apiKey = process.env.GLACIER_API_KEY;
  if (!walletAddress) throw new Error("Set WALLET_ADDRESS in your .env first.");
  if (!apiKey) throw new Error("Set GLACIER_API_KEY in your .env first. Get a free one at avacloud.io.");

  const avalanche = new Avalanche({ apiKey });

  const { result } = await avalanche.data.evm.address.transactions.listNative({
    chainId: "43113", // Fuji testnet
    address: walletAddress,
    pageSize: 10,
  });

  console.log(`Found ${result.transactions.length} transactions\n`);

  // The SDK returns nested, typed objects (from.address, txHash,
  // txStatus as a string), normalize() expects the flat shape shared
  // with direct-rpc.js, so map field names here rather than change the
  // shared normalizer.
  const rawTxs = result.transactions.map((tx) => ({
    hash: tx.txHash,
    value: tx.value,
    from: tx.from.address,
    to: tx.to.address,
    timestamp: tx.blockTimestamp,
    status: Number(tx.txStatus),
  }));

  const normalized = normalizeMany(rawTxs);
  for (const tx of normalized) {
    console.log(`${tx.status === "success" ? "OK" : "FAILED"}  ${tx.amount} ${tx.token}  ${tx.timestamp}  ${tx.hash}`);
  }
}

main().catch((err) => {
  console.error("ChainKit fetch error:", err.message);
  process.exit(1);
});
