package adminbackup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	// EncryptedFormat identifies an encrypted backup file
	EncryptedFormat = "go-uptime-admin-backup-encrypted"

	// LegacyEncryptedFormat identifies an encrypted backup file written while the project was called Gatus, up to v6. It
	// is still read and never written.
	LegacyEncryptedFormat = "gatus-admin-backup-encrypted"

	// The only key derivation parameters of version 1, the minimum recommended by OWASP for Argon2id, so that a forged
	// envelope cannot make the server derive a more expensive key
	kdfName      = "argon2id"
	kdfTime      = 2
	kdfMemoryKiB = 19456
	kdfThreads   = 1
	cipherName   = "aes-256-gcm"
	saltLength   = 16
	nonceLength  = 12
	keyLength    = 32

	// MinimumPasswordBytes and MaximumPasswordBytes bound the length of a password, in bytes of UTF-8
	MinimumPasswordBytes = 12
	MaximumPasswordBytes = 1024

	// maximumConcurrentDerivations and derivationWaitTimeout limit the memory and the CPU used by the key derivations
	maximumConcurrentDerivations = 2
	derivationWaitTimeout        = 5 * time.Second
)

var (
	// ErrInvalidPassword is returned when an encrypted backup cannot be decrypted, whether the password is wrong or the
	// file was changed
	ErrInvalidPassword = errors.New("invalid password or corrupted file")

	// ErrPasswordLength is returned when a password is too short or too long
	ErrPasswordLength = fmt.Errorf("the password must have between %d and %d bytes", MinimumPasswordBytes, MaximumPasswordBytes)

	// ErrPasswordRequired is returned when an encrypted backup is sent without password
	ErrPasswordRequired = errors.New("the backup file is encrypted: a password is required")

	// ErrNotEncrypted is returned when a password is sent with a backup file that is not encrypted
	ErrNotEncrypted = errors.New("the backup file is not encrypted: remove the password")

	// ErrBusy is returned when too many key derivations are in progress
	ErrBusy = errors.New("too many encrypted backups in progress, try again later")

	derivations = make(chan struct{}, maximumConcurrentDerivations)
)

// envelopeHeader is the header of an encrypted backup, whose JSON encoding, with the fields in this order, is the
// additional authenticated data of the encryption. It is not encrypted, but changing any of its fields makes the
// decryption fail.
type envelopeHeader struct {
	// Format identifies an encrypted backup file. It is "go-uptime-admin-backup-encrypted", or
	// "gatus-admin-backup-encrypted" in a file written up to v6.
	Format string `json:"format"`

	// Version is the version of the format of the envelope. It must be exactly 1.
	Version int `json:"version"`

	// KDF are the parameters of the derivation of the encryption key from the password.
	KDF envelopeKDF `json:"kdf"`

	// Cipher are the parameters of the encryption of the backup file.
	Cipher envelopeCipher `json:"cipher"`
}

// envelopeKDF are the key derivation parameters of an encrypted backup. Only the values written by Encrypt are
// accepted by Decrypt, so that a forged envelope cannot make the server derive a more expensive key.
type envelopeKDF struct {
	// Name is the key derivation function. It is always "argon2id".
	Name string `json:"name"`

	// Time is the number of passes of Argon2id. It is always 2.
	Time uint32 `json:"time"`

	// MemoryKiB is the memory used by Argon2id, in KiB. It is always 19456 (19 MiB).
	MemoryKiB uint32 `json:"memoryKiB"`

	// Threads is the parallelism of Argon2id. It is always 1.
	Threads uint8 `json:"threads"`

	// Salt is the random salt of the derivation, of 16 bytes, in standard base64 with padding. It is not secret.
	Salt string `json:"salt"`
}

// envelopeCipher are the encryption parameters of an encrypted backup
type envelopeCipher struct {
	// Name is the authenticated cipher. It is always "aes-256-gcm", with the 32-byte key derived from the password.
	Name string `json:"name"`

	// Nonce is the random nonce of the encryption, of 12 bytes, in standard base64 with padding. It is not secret.
	Nonce string `json:"nonce"`
}

// envelope is an encrypted backup file: the body of the response of POST /api/v1/admin/backup when a password of
// MinimumPasswordBytes (12) to MaximumPasswordBytes (1024) bytes is sent, and an accepted value of the file property of
// the restore routes, together with that password. The fields of the header are flattened into the same JSON object,
// and unknown fields are refused.
type envelope struct {
	envelopeHeader

	// Data is the encrypted backup file (a File encoded in JSON) followed by the 16-byte authentication tag of GCM, in
	// standard base64 with padding. It is the only encrypted part of the envelope, and it is refused when it decodes to
	// more than MaximumPlaintextBytes plus 64 bytes.
	Data string `json:"data"`
}

// CheckPassword checks the length of a password
func CheckPassword(password string) error {
	if len(password) < MinimumPasswordBytes || len(password) > MaximumPasswordBytes {
		return ErrPasswordLength
	}
	return nil
}

// acquireDerivation waits for a slot of the key derivations for at most derivationWaitTimeout
func acquireDerivation() (func(), error) {
	timer := time.NewTimer(derivationWaitTimeout)
	defer timer.Stop()
	select {
	case derivations <- struct{}{}:
		return func() { <-derivations }, nil
	case <-timer.C:
		return nil, ErrBusy
	}
}

func deriveKey(password string, salt []byte) ([]byte, error) {
	release, err := acquireDerivation()
	if err != nil {
		return nil, err
	}
	defer release()
	return argon2.IDKey([]byte(password), salt, kdfTime, kdfMemoryKiB, kdfThreads, keyLength), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt encrypts a backup file with a password, returning the envelope
func Encrypt(plaintext []byte, password string) ([]byte, error) {
	if err := CheckPassword(password); err != nil {
		return nil, err
	}
	salt, nonce := make([]byte, saltLength), make([]byte, nonceLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	header := envelopeHeader{
		Format:  EncryptedFormat,
		Version: Version,
		KDF:     envelopeKDF{Name: kdfName, Time: kdfTime, MemoryKiB: kdfMemoryKiB, Threads: kdfThreads, Salt: base64.StdEncoding.EncodeToString(salt)},
		Cipher:  envelopeCipher{Name: cipherName, Nonce: base64.StdEncoding.EncodeToString(nonce)},
	}
	additionalData, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, additionalData)
	return json.MarshalIndent(envelope{envelopeHeader: header, Data: base64.StdEncoding.EncodeToString(sealed)}, "", "  ")
}

// Decrypt decrypts an encrypted backup file. The parameters of the envelope must be exactly those of version 1, and a
// wrong password or a changed file both return ErrInvalidPassword.
func Decrypt(data []byte, password string) ([]byte, error) {
	var sealedEnvelope envelope
	if err := decodeStrict(data, &sealedEnvelope); err != nil {
		return nil, invalidFile("%s", err.Error())
	}
	header := sealedEnvelope.envelopeHeader
	// The header is authenticated as it was read: its format, the one of the file, is part of the additional data of the
	// seal, so the legacy format must not be rewritten before opening it
	if (header.Format != EncryptedFormat && header.Format != LegacyEncryptedFormat) || header.Version != Version {
		return nil, invalidFile("unsupported encrypted format %q version %d", header.Format, header.Version)
	}
	if header.KDF.Name != kdfName || header.KDF.Time != kdfTime || header.KDF.MemoryKiB != kdfMemoryKiB || header.KDF.Threads != kdfThreads || header.Cipher.Name != cipherName {
		return nil, invalidFile("unsupported encryption parameters")
	}
	salt, saltErr := base64.StdEncoding.Strict().DecodeString(header.KDF.Salt)
	nonce, nonceErr := base64.StdEncoding.Strict().DecodeString(header.Cipher.Nonce)
	sealed, dataErr := base64.StdEncoding.Strict().DecodeString(sealedEnvelope.Data)
	if saltErr != nil || nonceErr != nil || dataErr != nil || len(salt) != saltLength || len(nonce) != nonceLength {
		return nil, invalidFile("invalid salt, nonce or data")
	}
	if len(sealed) > MaximumPlaintextBytes+64 {
		return nil, invalidFile("the file exceeds %d bytes", MaximumPlaintextBytes)
	}
	if len(password) == 0 {
		return nil, ErrPasswordRequired
	}
	if err := CheckPassword(password); err != nil {
		return nil, ErrInvalidPassword
	}
	additionalData, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, sealed, additionalData)
	if err != nil {
		return nil, ErrInvalidPassword
	}
	return plaintext, nil
}

// Unwrap returns the plaintext of a backup file, encrypted or not, and whether it was encrypted. A password is required
// for an encrypted file and refused for a file without encryption.
func Unwrap(data []byte, password string) ([]byte, bool, error) {
	var detected struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(data, &detected); err != nil {
		return nil, false, invalidFile("the file is not a JSON object")
	}
	switch detected.Format {
	case EncryptedFormat, LegacyEncryptedFormat:
		plaintext, err := Decrypt(data, password)
		return plaintext, true, err
	case Format, LegacyFormat:
		if len(password) > 0 {
			return nil, false, ErrNotEncrypted
		}
		return data, false, nil
	default:
		return nil, false, invalidFile("unknown format %q", detected.Format)
	}
}
