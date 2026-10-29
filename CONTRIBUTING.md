# Contributing

1. Work on a branch; open a pull request against `main`.
2. Run `make check` before pushing (gofmt with the go.mod Go version,
   staticcheck, golangci-lint, vet for every release platform, govulncheck,
   the tests with the race detector and the 85 % coverage gate). On Linux,
   `make test-multicast` must pass too: it fails instead of skipping the
   routing tests when multicast loopback does not work.
3. New commands follow the existing shape: a `newXxxCmd(a *app)` constructor,
   validation of addresses, datapoint types and values before anything is
   sent, typed errors from `internal/errs`, a result type with `Text()` (and
   `Headers()`/`Rows()` for tables), streams through `app.stream()`.
4. Tests drive `cmd.Run` in process against a real go-knx server
   (`startServer` in `cmd/helpers_test.go`) or `knxctl serve`; no mocks of
   the protocol. Assert both the text and the JSON output and the exit code.
5. A command that sends telegrams says so in its short help ("transmits"),
   and is listed as such in the root help, README.md and docs/cli.md.
6. KNX logic (KNXnet/IP, cEMI, APDUs, datapoint types, the server) belongs in
   go-knx, not here.
7. Regenerate the reference after changing help texts: `make docs`
   (scripts/gen-cli-doc.sh). Update docs/json-output.md for new fields and
   docs/exitcodes.md for new error codes.
8. Add an SPDX header (`// SPDX-License-Identifier: MIT`) to new Go files.
9. Commit messages: imperative subject, a body that says why.
