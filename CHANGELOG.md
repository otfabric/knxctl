# Changelog

All notable changes are listed here; release notes are in
[RELEASE.md](RELEASE.md).

## v0.1.0 — 2026-10-29

First release: a command-line tool for KNX over KNXnet/IP on go-knx.
`discover` (multicast, extended and directed search), `describe`,
`diagnose`, `tunnel connect`; `monitor`, `group listen` and `routing
monitor` with datapoint decoding (`--map`, `--dpt`); `group write`, `group
respond`, `group read` (`--all`), `group answer`; offline `dpt list`,
`describe`, `encode`, `decode` and frame `decode` (KNXnet/IP and cEMI);
`serve`, a simulated KNXnet/IP server with virtual group objects
(`--object`, `--config`). Text, table and versioned JSON output, JSON Lines
streams, stable exit codes; `KNXCTL_FORMAT` and `KNXCTL_GATEWAY`.
