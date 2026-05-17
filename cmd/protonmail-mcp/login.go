package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func runLogin(
	ctx context.Context,
	apiURL string,
	transport http.RoundTripper,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	sess := session.New(apiURL, keychain.New(), session.WithTransport(transport))

	reader := bufio.NewReader(stdin)
	username, err := promptReader(stdout, reader, "Proton email: ")
	if err != nil {
		return err
	}
	password, err := readPassword(stdout, stdin, reader)
	if err != nil {
		return err
	}

	in := session.LoginInput{Username: username, Password: password}

	err = sess.Login(ctx, in)
	if err != nil && strings.Contains(err.Error(), "2FA required") {
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprintln(stdout, "2FA is enabled on this account.")
		_, _ = fmt.Fprintln(stdout,
			"Paste an otpauth:// URI (preferred — enables silent refresh) OR a 6-digit code.")
		v, err2 := promptReader(stdout, reader, "> ")
		if err2 != nil {
			return err2
		}
		if strings.HasPrefix(v, "otpauth://") {
			in.TOTPSecret = v
		} else if isAllDigits(v) && len(v) == 6 {
			in.TOTPCode = v
			_, _ = fmt.Fprintln(stdout,
				"WARNING: a one-shot code was provided. Future automatic refreshes will fail;"+
					" you'll need to log in again when the session expires.")
		} else {
			return fmt.Errorf(
				"2FA input invalid (expected otpauth:// URI or 6-digit code): %w", err)
		}
		err = sess.Login(ctx, in)
	}
	if err != nil {
		if pe := proterr.Map(err); pe != nil && pe.Code == "proton/captcha" {
			return fmt.Errorf("%s: %s\n%s", pe.Code, pe.Message, pe.Hint)
		}
		return err
	}

	_, _ = fmt.Fprintln(stdout, "Logged in. You can now run `protonmail-mcp status` to verify.")
	return nil
}

func promptReader(out io.Writer, r *bufio.Reader, label string) (string, error) {
	_, _ = fmt.Fprint(out, label)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	if errors.Is(err, io.EOF) && line == "" {
		return "", errors.New("unexpected EOF reading input")
	}
	return strings.TrimSpace(line), nil
}

func readPassword(out io.Writer, stdin io.Reader, fallback *bufio.Reader) (string, error) {
	_, _ = fmt.Fprint(out, "Password: ")
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		b, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	line, err := fallback.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	if errors.Is(err, io.EOF) && line == "" {
		return "", errors.New("unexpected EOF reading password")
	}
	return line, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
