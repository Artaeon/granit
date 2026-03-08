---
date: 2026-03-07
type: research
tags: [research, bitcoin, lightning-network, layer-2, scaling, payments]
source: https://coinlaw.io/bitcoin-lightning-network-usage-statistics/
---

## Bitcoin - Lightning Network and Scaling

Bitcoin's base layer processes approximately 7 transactions per second with ~10-minute block times. The Lightning Network is Bitcoin's primary Layer 2 scaling solution, enabling near-instant, low-cost payments by moving transactions off-chain.

## The Scaling Challenge

The Bitcoin [[Bitcoin - How It Works|blockchain]] has inherent throughput limitations by design:

- **Block size**: ~1-4 MB (with SegWit)
- **Block time**: ~10 minutes
- **Throughput**: ~7 TPS on-chain
- **Fees**: Can spike to $50+ during congestion

These constraints are a deliberate trade-off favoring decentralization and security over raw throughput. Scaling is achieved through layers rather than increasing base-layer capacity.

## How the Lightning Network Works

The Lightning Network operates through **payment channels**:

1. **Opening a channel**: Two parties lock bitcoin in a 2-of-2 multisig transaction on-chain
2. **Transacting**: They exchange signed transactions off-chain, updating balances instantly and with negligible fees
3. **Routing**: Payments can be routed through a network of channels (A -> B -> C) without requiring a direct channel between sender and receiver
4. **Closing a channel**: The final balance is settled on-chain in a single transaction

This allows theoretically unlimited transactions between channel opens/closes, with only the opening and closing transactions touching the blockchain.

## Network Statistics (2025-2026)

| Metric | Value |
|--------|-------|
| Public capacity | ~3,800-5,600 BTC (varies; private channels add more) |
| Monthly volume | Surpassed $1 billion in 2025 |
| Volume growth (2025) | ~300% year-over-year |
| Payment success rate | Approaching ~100% with multi-path payments |

## Key Technical Upgrades

Recent and ongoing improvements to Lightning:

- **Multi-Path Payments (MPP)**: Split large payments across multiple routes for higher success rates
- **Splicing**: Add or remove funds from a channel without closing it
- **Taproot integration**: Improved privacy and efficiency for channel operations
- **AMP (Atomic Multi-Path)**: Spontaneous payments across multiple paths
- **Taproot Assets**: Enables stablecoins (e.g., USDT) and other assets to be transacted over Lightning rails

## Taproot Assets -- Stablecoins on Lightning

In January 2025, Tether announced the launch of USDT on Bitcoin via the Taproot Assets protocol. This is significant because it:

- Brings fiat-denominated stablecoin payments to Lightning's speed and low fees
- Combines Bitcoin's security guarantees with the price stability of dollar-pegged assets
- Opens Lightning to remittance and commerce use cases where volatility was a barrier

## Real-World Adoption

- **Steak 'n Shake**: Reported a 50% reduction in payment processing fees after Lightning integration (May 2025) -- the largest retail Lightning adoption to date
- **Major exchanges**: Increasingly support Lightning deposits and withdrawals, bringing liquidity and users
- **Remittances**: Growing use in cross-border payments, particularly in Latin America and Africa
- Lightning could handle **over 30% of all BTC transfers** for payments and remittances by end of 2026

## Other Scaling Approaches

Beyond Lightning, other Bitcoin scaling solutions include:

- **Sidechains** (e.g., Liquid Network): Federated chains pegged to Bitcoin for faster settlement
- **Statechains**: Transfer UTXO ownership off-chain without opening channels
- **Ark**: A newer protocol for off-chain transactions with simpler UX than Lightning
- **Drivechain / BIP-300**: Proposed sidechain mechanism (controversial in the community)

## See Also

- [[Bitcoin - How It Works]]
- [[Bitcoin - Overview and History]]
- [[Bitcoin - Price History and Market]]
