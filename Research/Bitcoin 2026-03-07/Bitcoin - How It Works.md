---
date: 2026-03-07
type: research
tags: [research, bitcoin, blockchain, UTXO, transactions, technical]
source: https://learnmeabitcoin.com/technical/transaction/utxo/
---

## Bitcoin - How It Works

Bitcoin is a decentralized ledger system built on three core technical pillars: the blockchain, the UTXO model, and cryptographic signatures. Understanding these mechanics is essential to grasping why Bitcoin works without trusted third parties.

## The Blockchain

The Bitcoin blockchain is a linear chain of blocks, each containing a batch of validated transactions. Every block includes:

- **Block header**: Contains a hash of the previous block, a timestamp, the Merkle root of all transactions, the nonce, and the difficulty target
- **Transaction list**: All transactions included in that block
- **Previous block hash**: Creates an unbreakable chain back to the Genesis Block

Blocks are produced approximately every 10 minutes by [[Bitcoin - Mining and Consensus|miners]], and the chain is maintained by a global network of full nodes that independently validate every transaction and block.

## The UTXO Model

Unlike bank accounts that track running balances, Bitcoin uses the **Unspent Transaction Output (UTXO)** model:

1. When you receive bitcoin, the network creates a UTXO -- a discrete "chunk" of BTC locked to your address
2. When you spend bitcoin, your wallet selects one or more UTXOs as **inputs** and creates new UTXOs as **outputs**
3. Any difference between inputs and outputs becomes the **transaction fee**, collected by the miner

**Example**: If you have a 0.5 BTC UTXO and want to send 0.3 BTC:
- Input: 0.5 BTC UTXO (consumed and destroyed)
- Output 1: 0.3 BTC to the recipient (new UTXO)
- Output 2: 0.1997 BTC back to yourself as change (new UTXO)
- Fee: 0.0003 BTC to the miner

Each node maintains its own complete UTXO set and validates that inputs reference unspent outputs, preventing double-spending.

## Advantages of UTXO Over Account-Based Models

- **Privacy**: Each transaction can use fresh addresses, making tracing harder
- **Parallel validation**: UTXOs are independent, enabling concurrent verification
- **Auditability**: The full UTXO set can be independently reconstructed from the blockchain
- **No double-spending**: Each UTXO can only be consumed once

## Transactions and Scripting

Bitcoin transactions use a simple stack-based scripting language called **Script**. The most common transaction types are:

- **P2PKH** (Pay-to-Public-Key-Hash): The classic Bitcoin address format
- **P2SH** (Pay-to-Script-Hash): Enables multi-signature and more complex conditions
- **P2WPKH / P2WSH** (SegWit): Segregated Witness outputs that reduce transaction size
- **P2TR** (Taproot): Activated in November 2021, enables more private and efficient smart contracts using Schnorr signatures

## Cryptographic Foundations

- **SHA-256**: Used for block hashing and [[Bitcoin - Mining and Consensus|proof of work]]
- **ECDSA / Schnorr**: Digital signature algorithms for authorizing transactions
- **RIPEMD-160**: Used in address generation (hash of the public key)
- **Merkle Trees**: Efficiently summarize all transactions in a block, enabling lightweight verification (SPV)

## Network Architecture

Bitcoin operates as a peer-to-peer network with several node types:

- **Full nodes**: Store the complete blockchain and validate all rules (~24,700 reachable as of 2025)
- **Mining nodes**: Full nodes that also participate in [[Bitcoin - Mining and Consensus|block production]]
- **SPV/Light nodes**: Only download block headers; rely on full nodes for transaction verification
- **Lightning nodes**: Operate payment channels on the [[Bitcoin - Lightning Network and Scaling|Lightning Network]]

## See Also

- [[Bitcoin - Overview and History]]
- [[Bitcoin - Mining and Consensus]]
- [[Bitcoin - Lightning Network and Scaling]]
