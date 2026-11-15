// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/pflag"
)

// Secret flags (passwords and keys) are registered without a default
// value: the default of a flag is part of the usage text (--help,
// docs/cli.md), so it must never hold a secret. Their environment
// variables are read after the command line is parsed (resolveSecrets).
// Every secret flag has a twin --<name>-file, and its variable a twin
// <VARIABLE>_FILE, that read the value from a file.

// secretAnnotation marks a secret flag. Its values are the environment
// variable and the name of the -file flag.
const secretAnnotation = "knxctl_secret"

// maxSecretFileSize bounds a secret file, as the other input files are.
const maxSecretFileSize = 16 << 20

// secretFlag registers the secret flag name, its file twin name-file and
// the environment variables env and env_FILE. usage describes the value.
func secretFlag(f *pflag.FlagSet, p *string, name, env, usage string) {
	file := name + "-file"
	f.StringVar(p, name, "", fmt.Sprintf("%s (env %s); deprecated as a flag: use --%s or the variable", usage, env, file))
	f.String(file, "", fmt.Sprintf("Read --%s from `file`, without its trailing newline (env %s_FILE)", name, env))
	_ = f.SetAnnotation(name, secretAnnotation, []string{env, file})
}

// resolveSecrets sets every secret flag of the command that the command
// line did not give: from its -file flag, else from the environment.
func resolveSecrets(f *pflag.FlagSet) error {
	var err error
	f.VisitAll(func(fl *pflag.Flag) {
		ann, ok := fl.Annotations[secretAnnotation]
		if !ok || len(ann) != 2 || err != nil {
			return
		}
		err = resolveSecret(f, fl, ann[0], ann[1])
	})
	return err
}

// resolveSecret sets the secret flag fl: the flag, else its -file flag,
// else the variable env, else the file that env_FILE names. A flag and
// its -file flag, or both variables, are refused.
func resolveSecret(f *pflag.FlagSet, fl *pflag.Flag, env, file string) error {
	ff := f.Lookup(file)
	if ff == nil {
		return errs.New(errs.KindInternal, errs.CodeInternalUnexpected, "--%s has no --%s", fl.Name, file)
	}
	switch {
	case fl.Changed && ff.Changed:
		return errs.Invalid("--%s and --%s exclude each other", fl.Name, file)
	case fl.Changed:
		return nil
	case ff.Changed:
		v, err := readSecretFile("--"+file, ff.Value.String())
		if err != nil {
			return err
		}
		return fl.Value.Set(v)
	}
	v, path := os.Getenv(env), os.Getenv(env+"_FILE")
	switch {
	case v != "" && path != "":
		return errs.Invalid("%s and %s_FILE exclude each other", env, env)
	case path != "":
		s, err := readSecretFile(env+"_FILE", path)
		if err != nil {
			return err
		}
		v = s
	}
	if v == "" {
		return nil
	}
	return fl.Value.Set(v)
}

// readSecretFile reads one secret from a file; a trailing newline (LF or
// CRLF) is not part of it.
func readSecretFile(what, path string) (string, error) {
	b, err := readFileLimited(path, maxSecretFileSize)
	if err != nil {
		return "", errs.New(errs.KindInvalidInput, errs.CodeInvalidInput, "%s: %v", what, err)
	}
	s := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	if s == "" {
		return "", errs.Invalid("%s %s: the file is empty", what, path)
	}
	return s, nil
}
