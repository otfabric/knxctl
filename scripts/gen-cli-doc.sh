#!/bin/sh
# SPDX-License-Identifier: MIT
# Regenerates docs/cli.md from the --help of every command.
set -eu
cd "$(dirname "$0")/.."
GOWORK=off go build -o bin/knxctl .
# The reference must not depend on the caller's environment: no KNXCTL_
# variable reaches the binary.
for v in $(env | sed -n 's/^\(KNXCTL_[A-Za-z0-9_]*\)=.*/\1/p'); do
	unset "$v"
done
cmds="discover describe tunnel tunnel_connect tunnel_feature tunnel_feature_get tunnel_feature_set group group_write group_respond
group_read group_answer group_listen monitor routing routing_monitor diagnose
dpt dpt_list dpt_describe dpt_encode dpt_decode decode busmonitor property
property_read property_write keyring keyring_show serve version completion"
{
	echo "# knxctl command reference"
	echo
	echo "Generated from \`knxctl <command> --help\` by scripts/gen-cli-doc.sh (\`make docs\`); do not edit."
	echo "Exit codes: [exitcodes.md](exitcodes.md). JSON output: [json-output.md](json-output.md)."
	echo "Simulator configuration: [serve.md](serve.md)."
	echo
	echo '```text'
	bin/knxctl --help
	echo '```'
	for c in $cmds; do
		args=$(echo "$c" | tr _ ' ')
		echo
		echo "## knxctl $args"
		echo
		echo '```text'
		# shellcheck disable=SC2086
		bin/knxctl $args --help
		echo '```'
	done
} > docs/cli.md
