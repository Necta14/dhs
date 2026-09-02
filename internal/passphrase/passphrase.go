// Package passphrase învelește criptarea pachetului. Nu inventăm criptografie: folosim age
// (filippo.io/age) cu destinatar scrypt, proiectat și auditat exact pentru „un fișier, o parolă".
//
// Fraza goală înseamnă „fără criptare" și e o alegere explicită a utilizatorului (D4): fluxul trece
// neschimbat, iar antetul volumului spune Cipher none.
package passphrase

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
)

// WorkFactor e exponentul scrypt: 2^18 ≈ o secundă pe un laptop obișnuit la deschidere. Destul ca
// o parolă slabă să nu se spargă ieftin, puțin destul ca utilizatorul să nu aștepte.
const WorkFactor = 18

// MinLength e lungimea minimă acceptată. Nu suplinește o parolă bună, dar taie greșelile evidente.
const MinLength = 8

var (
	ErrTooShort = fmt.Errorf("passphrase: fraza de acces are sub %d caractere", MinLength)
	ErrWrong    = errors.New("passphrase: fraza de acces e greșită sau datele sunt corupte")
)

// Check validează o frază înainte să o folosim la scriere. La citire nu validăm: acceptăm ce a
// funcționat la scriere, oricât ar fi.
func Check(pass string) error {
	if len([]rune(strings.TrimSpace(pass))) < MinLength {
		return ErrTooShort
	}
	return nil
}

// Encrypt învelește dst într-un scriitor criptat. Trebuie închis (Close) ca ultimul bloc age să
// ajungă pe disc; fără Close, fluxul e trunchiat și nedecriptabil.
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

// Decrypt învelește src într-un cititor care decriptează. O frază greșită iese ca ErrWrong.
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
