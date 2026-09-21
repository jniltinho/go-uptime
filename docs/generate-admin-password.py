#!/usr/bin/env python3
"""Generate the security.basic.password-bcrypt-base64 value of Go Uptime, without htpasswd.

Since the binary has a command line, `go-uptime password hash` does the same without Python (see docs/cli.md). This script
stays for whoever does not have the binary at hand.

Equivalent to:

    htpasswd -bnBC 10 "" 'your-password' | tr -d ':\\n' | sed 's/$2y/$2a/' | base64 -w0 | tr '+/' '-_'

Usage:

    python3 docs/generate-admin-password.py                    # asks for the password without echoing it
    python3 docs/generate-admin-password.py --username admin   # also prints the config.yaml block
    printf '%s' 'your-password' | python3 docs/generate-admin-password.py --stdin

Uses the Python bcrypt module when it is installed (python3-bcrypt or pip install bcrypt). Without it, uses the pure
Python implementation of this file, which only needs Python 3 and takes a few seconds with the default cost (10).
"""

import argparse
import base64
import getpass
import os
import sys

# Base64 alphabet of bcrypt
BCRYPT_ALPHABET = b"./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

# Text encrypted 64 times by bcrypt, defined by the algorithm
BCRYPT_MAGIC = b"OrpheanBeholderScryDoubt"

MASK = 0xFFFFFFFF


def pi_words(count):
    """Return the first `count` 32-bit words of the hexadecimal fractional part of pi.

    They are the initial values of the Blowfish tables (P-array and S-boxes). Computing them with Machin's formula
    avoids copying 1042 constants by hand.
    """
    guard = 64
    precision = count * 32 + guard
    one = 1 << precision

    def arctan_inverse(x):
        term = one // x
        total = term
        x_squared = x * x
        n = 1
        sign = -1
        while term:
            term //= x_squared
            total += sign * (term // (2 * n + 1))
            sign = -sign
            n += 1
        return total

    pi = 16 * arctan_inverse(5) - 4 * arctan_inverse(239)
    fraction = (pi - (3 << precision)) >> guard
    return [(fraction >> (32 * (count - 1 - i))) & MASK for i in range(count)]


def cyclic_words(data, count):
    """Read `count` 32-bit words from `data`, wrapping around when the bytes run out (stream2word of bcrypt)."""
    words = []
    position = 0
    for _ in range(count):
        word = 0
        for _ in range(4):
            word = (word << 8) | data[position]
            position = (position + 1) % len(data)
        words.append(word)
    return words


def expand_state(p, s0, s1, s2, s3, key_words, data_words):
    """ExpandKey of EksBlowfish: mix the key into P and re-encrypt every table, with the optional data (the salt)."""
    for i in range(18):
        p[i] ^= key_words[i]
    left = right = 0
    position = 0
    for table, size in ((p, 18), (s0, 256), (s1, 256), (s2, 256), (s3, 256)):
        for index in range(0, size, 2):
            if data_words is not None:
                left ^= data_words[position]
                right ^= data_words[position + 1]
                position += 2
            for round_index in range(16):
                left ^= p[round_index]
                right ^= ((((s0[left >> 24] + s1[(left >> 16) & 255]) & MASK) ^ s2[(left >> 8) & 255]) + s3[left & 255]) & MASK
                left, right = right, left
            left, right = right ^ p[17], left ^ p[16]
            table[index] = left
            table[index + 1] = right


def encipher(p, s0, s1, s2, s3, left, right):
    """Encrypt a 64-bit block with the current Blowfish state."""
    for round_index in range(16):
        left ^= p[round_index]
        right ^= ((((s0[left >> 24] + s1[(left >> 16) & 255]) & MASK) ^ s2[(left >> 8) & 255]) + s3[left & 255]) & MASK
        left, right = right, left
    return right ^ p[17], left ^ p[16]


def bcrypt_base64(data):
    """Encode bytes with the base64 of bcrypt, without padding."""
    output = bytearray()
    i = 0
    while i < len(data):
        byte = data[i]
        i += 1
        output.append(BCRYPT_ALPHABET[byte >> 2])
        bits = (byte & 0x03) << 4
        if i >= len(data):
            output.append(BCRYPT_ALPHABET[bits])
            break
        byte = data[i]
        i += 1
        output.append(BCRYPT_ALPHABET[bits | (byte >> 4)])
        bits = (byte & 0x0F) << 2
        if i >= len(data):
            output.append(BCRYPT_ALPHABET[bits])
            break
        byte = data[i]
        i += 1
        output.append(BCRYPT_ALPHABET[bits | (byte >> 6)])
        output.append(BCRYPT_ALPHABET[byte & 0x3F])
    return bytes(output)


def bcrypt_python(password, salt, cost):
    """Compute the $2a$ bcrypt hash of `password` with a 16-byte salt, in pure Python."""
    words = pi_words(18 + 4 * 256)
    p = words[:18]
    s0, s1, s2, s3 = (words[18 + 256 * i:18 + 256 * (i + 1)] for i in range(4))
    # $2a$: the password ends with a zero byte and only the first 72 bytes count
    key = (password + b"\x00")[:72]
    key_words = cyclic_words(key, 18)
    salt_key_words = cyclic_words(salt, 18)
    expand_state(p, s0, s1, s2, s3, key_words, cyclic_words(salt, 18 + 4 * 256))
    for _ in range(1 << cost):
        expand_state(p, s0, s1, s2, s3, key_words, None)
        expand_state(p, s0, s1, s2, s3, salt_key_words, None)
    text = cyclic_words(BCRYPT_MAGIC, 6)
    for _ in range(64):
        for block in (0, 2, 4):
            text[block], text[block + 1] = encipher(p, s0, s1, s2, s3, text[block], text[block + 1])
    raw = b"".join(word.to_bytes(4, "big") for word in text)
    return b"$2a$%02d$" % cost + bcrypt_base64(salt) + bcrypt_base64(raw[:23])


def bcrypt_hash(password, cost, force_pure_python):
    """Return the $2a$ bcrypt hash of the password and the name of the implementation used."""
    if not force_pure_python:
        try:
            import bcrypt
        except ImportError:
            pass
        else:
            hashed = bcrypt.hashpw(password, bcrypt.gensalt(rounds=cost, prefix=b"2a"))
            if not bcrypt.checkpw(password, hashed):
                raise RuntimeError("the hash generated by the bcrypt module does not match the password")
            return hashed, "bcrypt module"
    print(f"Computing bcrypt in pure Python (cost {cost}), this can take a few seconds...", file=sys.stderr)
    return bcrypt_python(password, os.urandom(16), cost), "pure Python"


def read_password(from_stdin):
    if from_stdin:
        password = sys.stdin.read()
        # Accept a trailing line break, like the one of echo
        if password.endswith("\r\n"):
            password = password[:-2]
        elif password.endswith("\n"):
            password = password[:-1]
        return password
    password = getpass.getpass("Admin password: ")
    if getpass.getpass("Repeat the password: ") != password:
        sys.exit("The passwords do not match.")
    return password


def main():
    parser = argparse.ArgumentParser(description="Generate security.basic.password-bcrypt-base64 for the config.yaml of Go Uptime, without htpasswd.")
    parser.add_argument("--username", help="also print the security and admin blocks of config.yaml with this username")
    parser.add_argument("--cost", type=int, default=10, help="bcrypt cost, from 4 to 31 (default: 10, like htpasswd -C 10)")
    parser.add_argument("--stdin", action="store_true", help="read the password from the standard input instead of asking for it")
    parser.add_argument("--pure-python", action="store_true", help="do not use the bcrypt module, even if it is installed")
    args = parser.parse_args()

    if not 4 <= args.cost <= 31:
        sys.exit("The cost must be between 4 and 31.")
    password = read_password(args.stdin).encode("utf-8")
    if not password:
        sys.exit("The password cannot be empty.")
    if len(password) > 72:
        print("Warning: bcrypt only uses the first 72 bytes of the password.", file=sys.stderr)

    hashed, implementation = bcrypt_hash(password, args.cost, args.pure_python)
    # Base64 with the URL alphabet (with padding), which is how Go Uptime decodes the value
    value = base64.urlsafe_b64encode(hashed).decode("ascii")

    if args.username:
        print("security:")
        print("  basic:")
        print(f"    username: {args.username}")
        print(f'    password-bcrypt-base64: "{value}"')
        print()
        print("admin:")
        print("  enabled: true")
    else:
        print(value)
    print(f"(bcrypt cost {args.cost}, {implementation})", file=sys.stderr)


if __name__ == "__main__":
    main()
