# solas

## Goal

This project builds an api-extension server that uses dynamodb as a storage backend.
The aim is to run multiple kubernetes management clusters with no master of masters needed. Global state can be written to Custom reosurces
that get stored in dynamodb and replicated to all other member clusters. The inented global state is a pool of devices that can be leased.
clusters can join and leave the mesh. a device may only be leased to a single claim at a time. 

The following custom resources should prove the pattern:
Device : global state object that describes a devices and whose status shows whether it is climaed or not
DeviceClaim : cluster local claim on a resource

The result should be a fully working demo (it is fine to use localstack if possible)




## How work happens

1. **Spec first.** 
2. **Model in Quint.** 
3. **Cite with Duvet.** 
4. **Small issues.** Work is filed as GitHub issues an agent can finish in
   under five minutes. Each issue names its spec sentence, the exact task,
   the files, and the command that proves it is done. 

## Tests

- Unit tests run with the race detector: `devbox run test`.
- The simulator gate runs on every protocol change: `devbox run sim-gate`.
- End-to-end tests use `sigs.k8s.io/e2e-framework` against kind clusters,
  with plain `testing`. No Ginkgo. No envtest. `devbox run e2e`.
- Model-based tests replay Quint traces against the real API server.
- `devbox run verify` runs unit tests, Quint checks, and the Duvet gate.

## Tooling

Use devbox for everything. Do not install tools globally or call `nix-shell`
by hand. `devbox.json` lists the packages and the scripts above.
`devbox run duvet-install` installs Duvet through cargo on first use.


## Local resources

The development VM has 16 GB of memory. Running out of it kills the
Claude Code session, so heavy jobs must not overlap.

- devbox caps the Java heap at 3 GB (`JVM_ARGS` for Apalache,
  `JAVA_TOOL_OPTIONS` for TLC and other JVMs), Go package parallelism at
  four (`GOFLAGS=-p=4`), and the simulator gate at four shards.
- Run one heavy job at a time: a `quint verify`, a TLC run, an e2e run,
  or a race stress run. Do not start a second one in parallel.
- `devbox run e2e` takes a lock, so a second run waits for the first.
- A stress run uses one copy of `go test` with a moderate `-count`.
  Do not run many copies at once.
- Delete kind clusters when a run ends, and delete only clusters whose
  names start with `e2e-`.

## Architecture decisions already made

## Writing

Specs, issues, commit messages, and docs follow Simplified Technical
English: short sentences, one idea each, active voice, plain words. No
filler, no marketing adjectives, no emoji.

## Where things are

- `quint/` models; `scripts/quint-check.sh` positive and negative checks
- `.duvet/config.toml` and `.duvet/snapshot.txt` traceability gate
- `cmd/solas-apiserver/` the extension server; `demo/kind/` three-cluster demo
- `docs/confidence.md` the trust ladder; `docs/sim-gate.md` the simulator
- `.github/workflows/verify.yml` and `sim-gate.yml` the CI gates
