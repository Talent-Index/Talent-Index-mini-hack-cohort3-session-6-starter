// The Invoice Payment Agent, end to end, with full safety architecture.
//
// Five steps: define the condition (invoice overdue, unpaid), evaluate
// it against a mock invoice database, present the reasoning and get
// human approval, execute the payment as a real USDC transfer on Fuji,
// and log the outcome, approved or not. Wrapped in four required safety
// pillars: pre-flight checks, a spending limit, an idempotency check,
// and an audit log entry for every decision.
//
// See kill_switch.sol in this folder for the fourth pillar, the
// on-chain kill switch, that one lives in the smart contract, not this
// program.
//
// A note on the signing approach: there is no official ethers.js-style
// all-in-one SDK for Go that resolves cleanly in every network
// environment, the full go-ethereum client pulls in a very large
// dependency tree. This file uses only two focused, verified
// subpackages instead, go-ethereum's crypto (for Keccak256 hashing and
// ECDSA signing) and its rlp (for transaction encoding), then submits
// the signed transaction over plain JSON-RPC with eth_sendRawTransaction.
// Every piece here, key derivation, RLP encoding, signing, was compiled
// and run against a real test key during development, and cross-checked
// against the JavaScript, Python, and Rust versions in this repo, all
// four derive the identical wallet address from the same test key.
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/joho/godotenv"
)

const fujiRPC = "https://api.avax-test.network/ext/bc/C/rpc"
const fujiChainID = 43113

// Invoice mirrors the mock invoice database shared across all four
// languages in this repo.
type Invoice struct {
	ID         string
	Supplier   string
	AmountUSDC float64
	DueDate    time.Time
	Paid       bool
	Recipient  string
}

var invoices = []Invoice{
	{"INV-042", "Supplier X", 50, mustDate("2026-08-14"), false, "0xAb12cd34ef56ab12cd34ef56ab12cd34ef56ab12"},
	{"INV-043", "Supplier Y", 120, mustDate("2026-08-19"), false, "0xCd34ef56ab12cd34ef56ab12cd34ef56ab12cd34"},
	{"INV-044", "Supplier Z", 30, mustDate("2026-08-20"), true, "0xEf56ab12cd34ef56ab12cd34ef56ab12cd34ef56"},
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

var sentPaymentsLog = map[string]bool{} // idempotency, in memory for this demo

func maxPaymentUSDC() float64 {
	if v := os.Getenv("MAX_PAYMENT_USDC"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return 500 // spending limit, per transaction, default
}

func daysOverdue(due time.Time) int {
	return int(time.Since(due).Hours() / 24)
}

// Step 1 and 2: define the condition, evaluate it against the mock database.
func findOverdueInvoices() []Invoice {
	var overdue []Invoice
	for _, inv := range invoices {
		if !inv.Paid && daysOverdue(inv.DueDate) > 3 {
			overdue = append(overdue, inv)
		}
	}
	return overdue
}

// Safety pillar 1: pre-flight checks, run before any payment goes out,
// no exceptions. Safety pillar 2 (spending limit) and the idempotency
// half of pillar 1 are both checked here too.
func preflightChecks(inv Invoice) []string {
	var errors []string
	if inv.Recipient == "" || inv.Recipient == "0x0000000000000000000000000000000000000000" {
		errors = append(errors, "invalid recipient address")
	}
	if inv.AmountUSDC > maxPaymentUSDC() {
		errors = append(errors, fmt.Sprintf("amount exceeds spending limit of %.0f USDC", maxPaymentUSDC()))
	}
	if sentPaymentsLog[inv.ID] {
		errors = append(errors, "payment already sent for this invoice, idempotency check failed")
	}
	return errors
}

// Step 3: present reasoning, get human approval before anything executes.
func confirmPayment(inv Invoice, overdueDays int) bool {
	fmt.Printf("\nInvoice %s from %s is %d days overdue.\n", inv.ID, inv.Supplier, overdueDays)
	fmt.Printf("I intend to send %.0f USDC to %s.\n", inv.AmountUSDC, inv.Recipient)
	fmt.Print("Do you approve? (y/n): ")
	var answer string
	fmt.Scanln(&answer)
	return strings.EqualFold(strings.TrimSpace(answer), "y")
}

func rpcCall(method string, params []any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	resp, err := http.Post(fujiRPC, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", parsed.Error.Message)
	}
	return parsed.Result, nil
}

// erc20TransferData builds the calldata for transfer(address,uint256),
// the function selector 0xa9059cbb is the first four bytes of
// Keccak256("transfer(address,uint256)"), a standard, well-known ERC-20
// selector, followed by the recipient address and amount, each
// left-padded to 32 bytes per the Solidity ABI encoding rules.
func erc20TransferData(recipient string, amount *big.Int) []byte {
	selector := []byte{0xa9, 0x05, 0x9c, 0xbb}

	recipientBytes := make([]byte, 32)
	addrBytes := common_hexToBytes(recipient)
	copy(recipientBytes[32-len(addrBytes):], addrBytes)

	amountBytes := make([]byte, 32)
	amount.FillBytes(amountBytes)

	data := append(selector, recipientBytes...)
	data = append(data, amountBytes...)
	return data
}

func common_hexToBytes(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Step 4: on approval, actually execute the payment on Fuji. Builds a
// legacy transaction by hand, signs it with go-ethereum's crypto
// package, and broadcasts it with eth_sendRawTransaction.
func sendPayment(inv Invoice) (string, error) {
	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(os.Getenv("AGENT_PRIVATE_KEY"), "0x"))
	if err != nil {
		return "", fmt.Errorf("invalid AGENT_PRIVATE_KEY: %w", err)
	}
	fromAddr := crypto.PubkeyToAddress(privKey.PublicKey)

	nonceResult, err := rpcCall("eth_getTransactionCount", []any{fromAddr.Hex(), "latest"})
	if err != nil {
		return "", err
	}
	var nonceHex string
	json.Unmarshal(nonceResult, &nonceHex)
	nonce := new(big.Int)
	nonce.SetString(strings.TrimPrefix(nonceHex, "0x"), 16)

	// USDC on Fuji is an ERC-20, transfer() takes the recipient and an
	// amount in the token's smallest unit, 6 decimals for USDC, not 18
	// like AVAX, this trips people up constantly.
	amount := new(big.Int)
	amount.SetInt64(int64(inv.AmountUSDC * 1_000_000))
	data := erc20TransferData(inv.Recipient, amount)

	usdcAddr := common_hexToBytes(os.Getenv("FUJI_USDC_ADDRESS"))
	var to [20]byte
	copy(to[:], usdcAddr)

	gasPrice := big.NewInt(25_000_000_000) // 25 gwei, illustrative fixed gas price for this demo
	gasLimit := uint64(100000)

	// EIP-155 signing hash: RLP of [nonce, gasPrice, gas, to, value, data, chainId, 0, 0].
	signingRLP, err := rlp.EncodeToBytes([]any{
		nonce, gasPrice, gasLimit, to, big.NewInt(0), data, big.NewInt(fujiChainID), big.NewInt(0), big.NewInt(0),
	})
	if err != nil {
		return "", err
	}
	hash := crypto.Keccak256(signingRLP)

	sig, err := crypto.Sign(hash, privKey)
	if err != nil {
		return "", err
	}

	// crypto.Sign returns a 65-byte [R || S || V] signature with V as a
	// 0/1 recovery id. EIP-155 folds the chain id into V: v = recoveryId + chainId*2 + 35.
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	recoveryID := int64(sig[64])
	v := big.NewInt(recoveryID + fujiChainID*2 + 35)

	signedRLP, err := rlp.EncodeToBytes([]any{nonce, gasPrice, gasLimit, to, big.NewInt(0), data, v, r, s})
	if err != nil {
		return "", err
	}

	rawTxHex := "0x" + hex.EncodeToString(signedRLP)
	txHashResult, err := rpcCall("eth_sendRawTransaction", []any{rawTxHex})
	if err != nil {
		return "", err
	}
	var txHash string
	json.Unmarshal(txHashResult, &txHash)
	return txHash, nil
}

// Safety pillar 4 (the software half, the kill switch itself lives on
// chain, see kill_switch.sol): log every decision, approved or not.
func logDecision(invoiceID, reasoning string, approved bool, txHash string) {
	entry := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"invoiceId": invoiceID,
		"reasoning": reasoning,
		"approved":  approved,
		"txHash":    txHash,
	}
	encoded, _ := json.Marshal(entry)
	fmt.Println(string(encoded))
	// production: append to a durable log store, not just stdout
}

func main() {
	_ = godotenv.Load()
	_ = context.Background()

	overdue := findOverdueInvoices()

	for _, inv := range overdue {
		overdueDays := daysOverdue(inv.DueDate)
		reasoning := fmt.Sprintf("%s is %d days overdue, condition met (overdue > 3 days)", inv.ID, overdueDays)

		if errs := preflightChecks(inv); len(errs) > 0 {
			logDecision(inv.ID, fmt.Sprintf("Pre-flight failed: %s", strings.Join(errs, ", ")), false, "")
			continue
		}

		if !confirmPayment(inv, overdueDays) {
			logDecision(inv.ID, reasoning, false, "")
			continue
		}

		txHash, err := sendPayment(inv)
		if err != nil {
			log.Fatal("Payment agent error:", err)
		}
		sentPaymentsLog[inv.ID] = true
		logDecision(inv.ID, reasoning, true, txHash)
		fmt.Printf("Payment sent, transaction hash: %s\n", txHash)
	}
}
