package cmd

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

// maximumPasswordBytes is the limit of bcrypt, which silently ignores everything past it
const maximumPasswordBytes = 72

var (
	// errPasswordAsArgument never repeats what was typed: cobra.NoArgs answers `unknown command "<the password>"`, which
	// would print the password on the standard error, in the logs of a pipeline and wherever they are collected
	errPasswordAsArgument = errors.New("the password is never taken as an argument: it is read from the standard input")
	errEmptyPassword      = errors.New("the password is empty")
	errPasswordTooLong    = fmt.Errorf("the password has more than %d bytes, the limit of bcrypt", maximumPasswordBytes)
	errPasswordsMismatch  = errors.New("the two passwords are not the same")
)

var passwordCmd = &cobra.Command{
	Use:   "password",
	Short: "Work with the passwords of the configuration",
}

// passwordHashCmd never takes the password as an argument nor as a flag: it would stay in the history of the shell and
// in the list of processes
var passwordHashCmd = &cobra.Command{
	Use:   "hash",
	Short: "Hash a password for security.basic and for the login of a status page",
	Long: `Reads a password from the standard input and prints its bcrypt hash encoded in base64, the value of
security.basic.password-bcrypt-base64 and of auth.password-bcrypt-base64 of a status page.

On a terminal the password is asked twice, without echo. Through a pipe, one line is read and its line break is dropped:

    printf 'the-password' | go-uptime password hash`,
	Args: func(_ *cobra.Command, arguments []string) error {
		if len(arguments) > 0 {
			return errPasswordAsArgument
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		password, err := readPassword(cmd.InOrStdin(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		hash, err := hashPassword(password)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), hash)
		return nil
	},
}

func init() {
	passwordCmd.AddCommand(passwordHashCmd)
	rootCmd.AddCommand(passwordCmd)
}

// readPassword asks the password twice without echo on a terminal, and otherwise reads one line
func readPassword(input io.Reader, prompt io.Writer) (string, error) {
	if file, ok := input.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(prompt, "Password: ")
		first, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(prompt)
		if err != nil {
			return "", err
		}
		fmt.Fprint(prompt, "Repeat the password: ")
		second, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(prompt)
		if err != nil {
			return "", err
		}
		if string(first) != string(second) {
			return "", errPasswordsMismatch
		}
		return string(first), nil
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	// Exactly one line break goes, and nothing else: `echo password | go-uptime password hash` must not hash "password\n",
	// while spaces, and even a carriage return that is not part of the line break, belong to the password
	if trimmed, found := strings.CutSuffix(line, "\r\n"); found {
		return trimmed, nil
	}
	return strings.TrimSuffix(line, "\n"), nil
}

// hashPassword returns the bcrypt hash of the password in base64 with the URL alphabet, as the configuration decodes it
func hashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", errEmptyPassword
	}
	if len(password) > maximumPasswordBytes {
		return "", errPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(hash), nil
}
