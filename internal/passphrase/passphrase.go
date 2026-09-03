// Package passphrase wraps the package encryption. We do not invent cryptography: we use age
// (filippo.io/age) with an scrypt recipient, designed and audited precisely for "one file, one
// passphrase".
//
// An empty passphrase means "no encryption" and is an explicit choice of the user (D4): the
// stream passes through unchanged, and the volume header says Cipher none.
package passphrase

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
)

// WorkFactor is the scrypt exponent: 2^18 is roughly one second and 256 MiB on an ordinary
// laptop, at every open. Enough that a weak passphrase cannot be cracked cheaply, little enough
// that the user does not wait. It is a variable only so tests can lower it — a suite with dozens
// of small volumes would otherwise take whole minutes. The product never changes it. On read it
// does not matter: age takes the factor from the file header.
var WorkFactor = 18

// MinLength is the minimum accepted length. It is no substitute for a good passphrase, but it
// cuts off the obvious mistakes.
const MinLength = 8

var (
	ErrTooShort = fmt.Errorf("passphrase: passphrase is shorter than %d characters", MinLength)
	ErrWrong    = errors.New("passphrase: wrong passphrase or corrupt data")
)

// Check validates a passphrase before we use it for writing. On read we do not validate: we
// accept whatever worked at write time, however long it is.
func Check(pass string) error {
	if len([]rune(strings.TrimSpace(pass))) < MinLength {
		return ErrTooShort
	}
	return nil
}

// Encrypt wraps dst in an encrypting writer. It must be closed (Close) for the last age block to
// reach the disk; without Close the stream is truncated and undecryptable.
func Encrypt(dst io.Writer, pass string) (io.WriteCloser, error) {
	if pass == "" {
		return nopCloser{dst}, nil
	}
	r, err := age.NewScryptRecipient(pass)
	if err != nil {
		return nil, fmt.Errorf("passphrase: %w", err)
	}
	r.SetWorkFactor(WorkFactor)
	w, err := age.Encrypt(dst, r)
	if err != nil {
		return nil, fmt.Errorf("passphrase: %w", err)
	}
	return w, nil
}

// Decrypt wraps src in a decrypting reader. A wrong passphrase surfaces as ErrWrong.
func Decrypt(src io.Reader, pass string) (io.Reader, error) {
	if pass == "" {
		return src, nil
	}
	id, err := age.NewScryptIdentity(pass)
	if err != nil {
		return nil, fmt.Errorf("passphrase: %w", err)
	}
	r, err := age.Decrypt(src, id)
	if err != nil {
		var incorrect *age.NoIdentityMatchError
		if errors.As(err, &incorrect) {
			return nil, ErrWrong
		}
		return nil, fmt.Errorf("passphrase: %w", err)
	}
	return r, nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
