// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package push

import (
	"crypto/rand"
	"math/big"
)

// GeneratedTokenLength is the length of the tokens generated for push keys and push endpoints, like the Uptime Kuma
const GeneratedTokenLength = 32

const tokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// GenerateToken returns a token of GeneratedTokenLength letters and digits from a cryptographic random generator
func GenerateToken() (string, error) {
	token := make([]byte, GeneratedTokenLength)
	alphabetLength := big.NewInt(int64(len(tokenAlphabet)))
	for i := range token {
		index, err := rand.Int(rand.Reader, alphabetLength)
		if err != nil {
			return "", err
		}
		token[i] = tokenAlphabet[index.Int64()]
	}
	return string(token), nil
}
