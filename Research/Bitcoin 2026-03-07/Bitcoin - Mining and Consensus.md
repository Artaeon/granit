---
date: 2026-03-07
type: research
tags: [research, bitcoin, mining, proof-of-work, consensus, hashrate]
source: https://cointelegraph.com/explained/bitcoin-mining-in-2025
---

## Bitcoin - Mining and Consensus

Bitcoin mining is the process by which new blocks are added to the blockchain and new bitcoins are issued. It serves as the consensus mechanism that allows a decentralized network to agree on a single transaction history without a central authority.

## Proof of Work (PoW)

Miners compete to solve a computational puzzle -- finding a nonce value that, when hashed with the block header using SHA-256, produces a hash below the current difficulty target. Key properties:

- **Costly to produce**: Requires significant computational work and electricity
- **Cheap to verify**: Any node can verify a valid hash in microseconds
- **Probabilistic**: Finding a valid hash is essentially a lottery; more hashpower = more chances
- **Sybil-resistant**: An attacker cannot create fake identities to gain influence; only real computational work counts

## Difficulty Adjustment

Bitcoin automatically adjusts mining difficulty every **2,016 blocks** (~2 weeks) to maintain an average block time of ~10 minutes:

- If blocks are found faster than 10 minutes, difficulty increases
- If blocks are found slower, difficulty decreases
- This self-regulating mechanism ensures consistent block production regardless of total hashrate

As of early 2026, difficulty exceeds **110 trillion**, a testament to the enormous computational power securing the network.

## Network Hashrate

The total computational power of the Bitcoin network has grown exponentially:

| Period | Hashrate |
|--------|----------|
| 2015 | ~400 PH/s |
| 2020 | ~120 EH/s |
| 2024 | ~600 EH/s |
| Sep 2025 | 1,120 EH/s (record) |
| Early 2026 | ~800+ EH/s |

The hashrate peaked at **1.12 billion TH/s (1,120 EH/s)** in September 2025, making Bitcoin the most computationally secured network in existence.

## Mining Hardware Evolution

Bitcoin mining hardware has progressed through four generations:

1. **CPUs** (2009-2010): Satoshi mined the first blocks on a standard computer
2. **GPUs** (2010-2013): Graphics cards offered ~100x improvement over CPUs
3. **FPGAs** (2011-2013): Field-programmable gate arrays offered better efficiency
4. **ASICs** (2013-present): Application-Specific Integrated Circuits dominate today

Modern ASICs achieve efficiency below **15 J/TH** (joules per terahash), a ~7x improvement from 2018's ~98 J/TH. Only ASIC miners are economically viable for Bitcoin mining since ~2013.

## Block Rewards

Miners receive two types of compensation:

1. **Block subsidy**: Currently **3.125 BTC** per block (since the April 2024 [[Bitcoin - Halving Cycles and Supply|halving]])
2. **Transaction fees**: Sum of all fees from transactions included in the block

As the block subsidy decreases with each [[Bitcoin - Halving Cycles and Supply|halving]], transaction fees are expected to become an increasingly important revenue source for miners.

## Mining Pools

Individual miners join mining pools to combine hashrate and share rewards proportionally. Major pools as of 2025-2026 include Foundry USA, AntPool, F2Pool, and ViaBTC. Pool mining reduces income variance for individual participants.

## Energy Considerations

Bitcoin mining is energy-intensive by design -- this cost is what makes the network secure. Key points:

- The network consumes an estimated 120-180 TWh annually
- An increasing share comes from renewable sources (estimated 50-60% as of 2025)
- Mining incentivizes development of stranded energy resources and grid stabilization
- The energy cost per transaction is misleading, as mining secures the entire UTXO set, not individual transactions

## See Also

- [[Bitcoin - How It Works]]
- [[Bitcoin - Halving Cycles and Supply]]
- [[Bitcoin - Overview and History]]
