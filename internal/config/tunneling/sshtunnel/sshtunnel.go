// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package sshtunnel holds the configuration of one SSH tunnel of the tunneling section of the YAML configuration and
// implements the tunnel itself: a lazily established SSH connection through which the checks dial their target.
package sshtunnel

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Config represents the configuration for an SSH tunnel
type Config struct {
	Type       string `yaml:"type"`                  // Type of the tunnel; only "SSH" is supported
	Host       string `yaml:"host"`                  // Host is the address of the SSH server; required
	Port       int    `yaml:"port,omitempty"`        // Port of the SSH server; defaults to 22
	Username   string `yaml:"username"`              // Username to log in with; required
	PrivateKey string `yaml:"private-key,omitempty"` // PrivateKey in PEM format; used instead of Password when both are set
	Password   string `yaml:"password,omitempty"`    // Password of the user; required when there is no PrivateKey
}

// ValidateAndSetDefaults validates the SSH tunnel configuration and sets defaults: the port defaults to 22. It returns
// an error if the type is not "SSH", if the host or the username is missing, or if there is neither a private key nor
// a password.
func (c *Config) ValidateAndSetDefaults() error {
	if c.Type != "SSH" {
		return fmt.Errorf("unsupported tunnel type: %s", c.Type)
	}
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	if c.PrivateKey == "" && c.Password == "" {
		return fmt.Errorf("either private-key or password is required")
	}
	if c.Port == 0 {
		c.Port = 22
	}
	return nil
}

// SSHTunnel represents an SSH tunnel connection
type SSHTunnel struct {
	config *Config
	mu     sync.RWMutex
	client *ssh.Client

	// Cached authentication methods to avoid reparsing private keys
	authMethods []ssh.AuthMethod
}

// New creates a new SSH tunnel with the given configuration, without connecting. A private key that cannot be parsed
// is not reported here: it makes the first connection attempt fail.
func New(config *Config) *SSHTunnel {
	tunnel := &SSHTunnel{
		config: config,
	}
	// Parse authentication methods once during initialization to avoid
	// expensive cryptographic operations on every connection attempt
	if config.PrivateKey != "" {
		if signer, err := ssh.ParsePrivateKey([]byte(config.PrivateKey)); err == nil {
			tunnel.authMethods = []ssh.AuthMethod{ssh.PublicKeys(signer)}
		}
		// Note: We don't return error here to maintain backward compatibility.
		// Invalid keys will be caught during first connection attempt.
	} else if config.Password != "" {
		tunnel.authMethods = []ssh.AuthMethod{ssh.Password(config.Password)}
	}
	return tunnel
}

// Connect establishes the SSH connection, with a timeout of 30 seconds and without verifying the host key. It
// returns an error if no authentication method is available or if the connection fails.
func (t *SSHTunnel) Connect() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connectUnsafe()
}

// connectUnsafe establishes the SSH connection without acquiring locks
// Must be called with t.mu.Lock() already held
func (t *SSHTunnel) connectUnsafe() error {
	// Use cached authentication methods to avoid expensive crypto operations
	if len(t.authMethods) == 0 {
		return fmt.Errorf("no authentication method available")
	}
	config := &ssh.ClientConfig{
		User:            t.config.Username,
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Skip host key verification
		Auth:            t.authMethods,               // Use pre-parsed authentication
	}
	// Connect to SSH server
	addr := fmt.Sprintf("%s:%d", t.config.Host, t.config.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	t.client = client
	return nil
}

// Close closes the SSH connection, if there is one. The tunnel can be used again: Dial reconnects.
func (t *SSHTunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client != nil {
		err := t.client.Close()
		t.client = nil
		return err
	}
	return nil
}

// Dial creates a connection through the SSH tunnel, connecting the tunnel first if needed. A failed dial is retried up
// to 3 times in total, reconnecting the tunnel after a backoff of 500ms and then 1s.
func (t *SSHTunnel) Dial(network, addr string) (net.Conn, error) {
	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()
	// Ensure we have an SSH connection
	if client == nil {
		// Use write lock to prevent race condition during connection
		t.mu.Lock()
		// Double-check client after acquiring lock
		if t.client == nil {
			if err := t.connectUnsafe(); err != nil {
				t.mu.Unlock()
				return nil, err
			}
		}
		client = t.client
		t.mu.Unlock()
	}
	// Attempt dial with exponential backoff retry
	const maxRetries = 3
	const baseDelay = 500 * time.Millisecond
	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s, 2s
			delay := baseDelay << (attempt - 1)
			time.Sleep(delay)
			// Close stale connection and reconnect
			t.mu.Lock()
			if t.client != nil {
				_ = t.client.Close()
				t.client = nil
			}
			if err := t.connectUnsafe(); err != nil {
				t.mu.Unlock()
				lastErr = fmt.Errorf("reconnect attempt %d failed: %w", attempt, err)
				continue
			}
			client = t.client
			t.mu.Unlock()
		}
		conn, err := client.Dial(network, addr)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("SSH tunnel dial failed after %d attempts: %w", maxRetries, lastErr)
}
