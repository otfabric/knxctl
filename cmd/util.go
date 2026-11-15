// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/otfabric/knxctl/internal/errs"
)

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// addDuration adds two non-negative durations, saturating at the largest
// duration instead of wrapping to a negative one (an expired deadline).
func addDuration(a, b time.Duration) time.Duration {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// checkCount rejects a negative --count (0 is no limit).
func checkCount(flag string, n int) error {
	if n < 0 {
		return errs.Invalid("--%s %d: must be 0 (no limit) or positive", flag, n)
	}
	return nil
}

// checkDuration rejects a negative duration flag (0 is no limit).
func checkDuration(flag string, d time.Duration) error {
	if d < 0 {
		return errs.Invalid("--%s %v: must be 0 (no limit) or positive", flag, d)
	}
	return nil
}

// decodeHex decodes hexadecimal octets as a user writes them: an optional
// 0x or 0X prefix, spaces between the octets.
func decodeHex(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, " ", "")
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
	}
	return hex.DecodeString(s)
}

// termSafe returns s with every character escaped that a terminal would
// act on or display misleadingly, as strconv.Quote escapes it but without
// the quotes: C0 and C1 controls, DEL, Unicode format characters (the
// bidirectional controls among them) and other non-printable runes, and
// invalid UTF-8. A backslash is doubled, so the original stays
// recoverable. Strings from peers and files (a server's friendly name, a
// keyring's creator) go through it before table and text output.
func termSafe(s string) string {
	safe := true
	for _, r := range s {
		if r == '\\' || r == utf8.RuneError || !strconv.IsPrint(r) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\\':
			b.WriteString(`\\`)
		case strconv.IsPrint(r):
			b.WriteString(s[i : i+n])
		default:
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		}
		i += n
	}
	return b.String()
}

// Input files (keyrings, configurations) are small; a larger file is not
// one, and reading it whole (a device, a FIFO fed forever) would exhaust
// memory.
const (
	maxKeyringSize = 16 << 20
	maxConfigSize  = 16 << 20
)

// readFileLimited reads a file of at most limit octets.
func readFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: larger than %d MiB", path, limit>>20)
	}
	return b, nil
}
