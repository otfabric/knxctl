#!/bin/sh
# SPDX-License-Identifier: MIT
# Regenerates docs/cli.md from the --help of every command.
set -eu
cd "$(dirname "$0")/.."
GOWORK=off go build -o bin/knxctl .
cmds="discover describe tunnel tunnel_connect group group_write group_respond
group_read group_answer group_listen monitor routing routing_monitor diagnose
dpt dpt_list dpt_describe dpt_encode dpt_decode decode serve version completion"
{
	echo "# knxctl command reference"
	echo
	echo "Generated from \`knxctl <command> --help\` by scripts/gen-cli-doc.sh (\`make docs\`); do not edit."
	echo "Exit codes: [exitcodes.md](exitcodes.md). JSON output: [json-output.md](json-output.md)."
	echo "Simulator configuration: [serve.md](serve.md)."
	echo
	echo '```text'
	KNXCTL_FORMAT='' KNXCTL_GATEWAY='' bin/knxctl --help
	echo '```'
	for c in $cmds; do
		args=$(echo "$c" | tr _ ' ')
		echo
		echo "## knxctl $args"
		echo
		echo '```text'
		# shellcheck disable=SC2086
		KNXCTL_FORMAT='' KNXCTL_GATEWAY='' bin/knxctl $args --help
		echo '```'
	done
} > docs/cli.md
