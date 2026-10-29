// SPDX-License-Identifier: MIT

package cmd

import "time"

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }
