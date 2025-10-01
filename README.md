# Validator Selection Simulation Toolkit

This repository contains the code and automation used to reproduce the
experiments from the paper **“A Game theoretic approach for validator selection
in proof of stake blockchains.”** The authors propose a proof-of-stake consensus
variant that combines a weighted lottery with a Vickrey (second-price) auction to
encourage truthful staking and limit validator monopolies. The code here mirrors
that workflow and provides scripts to run the same comparative simulations that
appear in the publication.

---

## Contents

- `Vic_gen/`, `Vick/` – Vickrey-auction validators with weighted random
  selection. `Vic_gen` uses an exact Gini implementation; `Vick` uses a faster
  approximation to match the paper’s baseline experiments.
- `Random_gen/`, `Random/` – Control variants where validators are chosen purely
  by stake-weighted randomness (no second-price auction). Again, `_gen` uses the
  exact Gini metric while `_random` mirrors the heuristic used in the study.
- `tools/client/` – Go-based validator simulator that reproduces the automated
  bidding behaviour discussed in the paper.
- `run_experiments.sh` – Orchestrates servers and simulated validators, captures
  logs, and archives blockchain snapshots for later analysis.
- `vendor/` – Lightweight, in-repo stand-ins for the third-party libraries named
  in the paper (spew, godotenv, xlsx) so that everything builds offline.

Each server instance follows the paper’s workflow:

1. Load `PORT` from `.env` (default 8080).
2. Initialise a PoS blockchain with a genesis block.
3. Accept TCP connections from validators, who register their initial stake.
4. Collect bids for each round, run either the Vickrey auction or the random
   lottery, announce the winner, and append a block.
5. Calculate the Gini coefficient after every block to quantify balance
   dispersion – the key fairness metric in the paper.
6. Export the blockchain history so that the bidding dynamics can be analysed.

---

## Prerequisites

- Go toolchain (1.13 or newer). The repo includes vendored replacements for
  `github.com/davecgh/go-spew/spew`, `github.com/joho/godotenv`, and
  `github.com/tealeg/xlsx`, so no network fetches are required once Go is
  installed.
- Bash (for the automation script).
- `python3` (used by `run_experiments.sh` to locate free TCP ports).

All build artifacts and caches are kept inside the repository (`.gocache`,
`.gopath`) so the toolchain will not modify files outside `Simulation/`.

---

## Quick Start: Reproducing the Paper’s Experiments

The paper compares the proposed Vickrey-based selection (`Vic_gen`) with a
stake-weighted random baseline (`Random`). To run both experiments back-to-back:

```bash
cd ~/Desktop/Simulation
./run_experiments.sh --variant Vic_gen --variant Random --duration 600 --clients 12
```

What this does:

1. Builds each server and the client simulator with repo-local `GOCACHE` and
   `GOPATH` values.
2. Reads the port from `variant/.env`; if it is already in use the script
   automatically selects a free port and sets the `PORT` environment variable for
   both server and clients. This prevents the “address already in use” failures
   you might have seen when running experiments manually.
3. Launches the selected server variant, waits for it to log
   `TCP Server Listening`, then starts the client simulator with the requested
   number of validators, balances, and bidding cadence.
4. After the specified duration (600 seconds above) the simulator exits, the
   server is terminated cleanly, and the exported blockchain snapshot is copied
   into `artifacts/`.

Artifacts are timestamped for traceability, for example:

- `artifacts/20251001-122511_Vic_gen_server.log`
- `artifacts/20251001-122511_Vic_gen_clients.log`
- `artifacts/20251001-122511_Vic_gen_blockchain.txt`

The `*_blockchain.txt` files contain the block index, timestamp, proposer,
winning validator, and the second-price transfer – mirroring the tables produced
in the study.

### Adjustable parameters

`run_experiments.sh` accepts the following options:

| Flag | Meaning | Default |
| ---- | ------- | ------- |
| `--variant` | Which simulator to run (`Vic_gen`, `Vick`, `Random_gen`, `Random`) | `Vic_gen`, `Random` |
| `--duration` | Experiment length in seconds | `300` |
| `--clients` | Number of simulated validators | `10` |
| `--balance` | Starting balance per validator | `1000` |
| `--base-cost` | Baseline bid magnitude | `15` |
| `--round` | Seconds between bids from the same validator | `60` |
| `--inter-delay` | Delay between BPM and bid submissions | `1` |

Environment variables `CACHE_DIR`, `GOPATH_DIR`, and `ARTIFACT_DIR` can be set
to override where the script stores build artifacts and logs.

---

## Manual Control of Validators

The automation script is perfect for reproducing the published data, but the
paper also discusses how individual validators strategise. You can imitate that
behaviour in two ways:

### 1. Pre-built Go client per validator

Build the client simulator once:

```bash
cd ~/Desktop/Simulation
mkdir -p tools/client-build
GOCACHE="$(pwd)/.gocache" GOPATH="$(pwd)/.gopath" \
    go build -o tools/client-build/client ./tools/client
```

Then, in separate terminals (one per validator), run:

```bash
cd ~/Desktop/Simulation/tools/client-build
./client --host 127.0.0.1 --port 8080 --balance 1200 --seed 135 --duration 600
```

Tune the flags per validator:

- `--balance`: starting stake for that validator.
- `--seed`: different seeds create different bidding personalities (overbidding
  streaks, timing, etc.) while following the rules described in the paper.
- `--round`, `--inter-delay`, `--base-cost`, `--bpm-min`, `--bpm-max`: adjust the
  pace and aggressiveness of bids.

Because the client drains server announcements in the background, the terminal
stays mostly quiet; when a run ends you will see `[client-X] completed`.

### 2. Fully manual (nc/telnet)

For complete control of every bid, connect with your favourite TCP client:

```bash
nc 127.0.0.1 8080
```

Respond to the prompts:

1. `Enter token balance:` – type the starting stake.
2. `Enter a new BPM:` – enter a BPM value (e.g. 72).
3. `Submit your bid:` – type the tokens you are staking for that block.

Open multiple terminals to mimic several validators, manually varying their
stakes and bids to observe how the Vickrey auction handles different scenarios.

---

## Interpreting Results

- **Server logs**: show weighted lottery winners, second-price transfers, and the
  Gini coefficient after each block. In the Vickrey variant you should see the
  balance disparity grow more slowly than in the random variant, matching the
  paper’s findings.
- **Client logs**: record dial errors, any network issues, and a completion
  message for each validator instance.
- **Blockchain snapshots**: the `blockchain.txt` export is a plain-text stand-in
  for an XLSX workbook. It records the same columns described in the paper
  (Index, Timestamp, Hashes, Validator Address, Proposer Address, Transfer) and
  can be imported into spreadsheets for plotting or additional analysis.


---

## Notes on the Offline Dependencies

- `vendor/github.com/davecgh/go-spew`: provides `spew.Dump` for the genesis block
  diagnostic output.
- `vendor/github.com/joho/godotenv`: implements the small subset of `.env` file
  parsing used by the simulators. It honours existing environment variables, so
  `PORT` can still be overridden externally.
- `vendor/github.com/tealeg/xlsx`: emits a readable text file instead of a real
  XLSX workbook. The API matches the original library so that replacing the stub
  with the full dependency later only requires dropping the real module in place.

If you later restore network access, simply remove the corresponding entries in
`go.mod` and run `go mod tidy` to fetch the official libraries.

---

## Troubleshooting

- **Address already in use**: the `run_experiments.sh` checks for busy
  ports and automatically selects a free one. If you start servers manually, set
  `PORT` to a free value before running `go run .`.
- **No client activity**: the Go client simulator handles server prompts
  silently. Check the server log for errors; on success it will print the Gini
  line every minute. If you want interactive feedback, use the manual `nc`
  method instead.
- **Missing artifacts**: ensure the server has permission to write inside its
  directory. The automation script copies `blockchain.xlsx` into `artifacts/`
  after every run; if the export is missing you’ll see a warning in the console.


---

## Citation
`
@INPROCEEDINGS{10390962,
  author={Rao, Balaji and Hayeri, Yeganeh M.},
  booktitle={2023 International Conference on Artificial Intelligence, Blockchain, Cloud Computing, and Data Analytics (ICoABCD)}, 
  title={A Game theoretic approach for validator selection in proof of stake blockchains}, 
  year={2023},
  volume={},
  number={},
  pages={31-36},
  keywords={Data analysis;Design methodology;Estimation;Games;Blockchains;Peer-to-peer computing;Consensus protocol;blockchain;proof-of-stake;consensus mechanisms;validator selection;game theory},
  doi={10.1109/ICoABCD59879.2023.10390962}}
`
