package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

var (
	_ PasswordReader    = (*StdinPasswordReader)(nil)
	_ PasswordReader    = (*FilePasswordReader)(nil)
	_ PasswordPrompter  = (*WriterPasswordPrompter)(nil)
	_ PasswordConfirmer = (*WriterPasswordConfirmer)(nil)
)

// PasswordReader can read a password from a password source.
type PasswordReader interface {
	ReadPassword() (password string, err error)
}

// StdinPasswordReader is a PasswordReader that can read a password from
// os.Stdin.
type StdinPasswordReader struct{}

// FilePasswordReader is a PasswordReader that can read a password stored in
// a file.
type FilePasswordReader struct {
	path string
}

// NewStdinPasswordReader creates a StdinPasswordReader.
func NewStdinPasswordReader() *StdinPasswordReader { return &StdinPasswordReader{} }

// NewFilePasswordReader creates a FilePasswordReader.
func NewFilePasswordReader(path string) *FilePasswordReader {
	return &FilePasswordReader{path}
}

// ReadPassword implements the PasswordReader interface. It reads the password
// from os.Stdin.
func (pr *StdinPasswordReader) ReadPassword() (password string, err error) {
	b, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		err = fmt.Errorf("read password: %w", err)
		return
	}
	password = string(b)
	return
}

// ReadPassword implements the PasswordReader interface. It opens the
// configured file and reads the password from it, trimming any newlines found.
//
// NOTE: This will read the entire password file into memory.
func (pr *FilePasswordReader) ReadPassword() (password string, err error) {
	b, err := ioutil.ReadFile(pr.path)
	if err != nil {
		err = fmt.Errorf("read password: %w", err)
		return
	}
	password = string(bytes.Trim(b, "\n"))
	return
}

// PasswordPrompter can prompt the user with a message to enter a password, and
// then read it.
type PasswordPrompter interface {
	PromptPassword(msg string) (password string, err error)
}

// WriterPasswordPrompter is a PasswordPrompter that prompts the user through a
// io.Writer.
type WriterPasswordPrompter struct {
	out      io.Writer
	pwReader PasswordReader
}

// PasswordPrompterOption is a function that applies an option to a
// FilePasswordPrompter.
type PasswordPrompterOption func(*WriterPasswordPrompter)

// NewPasswordPrompter creates a new FilePasswordPrompter with the given
// options. Prompt is written to os.Stdout by default
func NewPasswordPrompter(opts ...PasswordPrompterOption) (fpp *WriterPasswordPrompter) {
	fpp = &WriterPasswordPrompter{}
	for _, o := range opts {
		o(fpp)
	}
	if fpp.out == nil {
		fpp.out = os.Stdout
	}
	return
}

// PromptPassword implements PasswordPrompter. It writes a prompt message to
// the out file (default os.Stdout) and reads the password throug the
// configured password reader.
func (fpp *WriterPasswordPrompter) PromptPassword(msg string) (password string, err error) {
	fmt.Fprint(fpp.out, msg)
	return fpp.pwReader.ReadPassword()
}

// PasswordConfirmer prompts the user with a message to enter a password, then
// prompts again to confirm it.
type PasswordConfirmer interface {
	PromptConfirmPassword(msg, confimMsg string) (password string, err error)
}

// WriterPasswordConfirmer is a PasswordConfirmer that prompts the user through a
// io.Writer.
type WriterPasswordConfirmer struct {
	out        io.Writer
	pwPrompter PasswordPrompter
}

// PasswordConfirmerOption is a function that applies an option to a
// FilePasswordConfirmer.
type PasswordConfirmerOption func(*WriterPasswordConfirmer)

// NewPasswordConfirmer will create a FilePasswordConfirmer with the given
// options. Prompt is written to os.Stdout by default.
func NewPasswordConfirmer(opts ...PasswordConfirmerOption) (fpc *WriterPasswordConfirmer) {
	fpc = &WriterPasswordConfirmer{}
	for _, o := range opts {
		o(fpc)
	}
	if fpc.out == nil {
		fpc.out = os.Stdout
	}
	return
}

// PromptConfirmPassword implements the PasswordConfirmer interface. It prompts
// the user to enster a password and then to confirm it. Returns an error if
// the passwords are not equal.
func (fpc *WriterPasswordConfirmer) PromptConfirmPassword(msg, confirmMsg string) (password string, err error) {
	password, err = fpc.pwPrompter.PromptPassword(msg)
	if err != nil {
		return
	}
	pass2, err := fpc.pwPrompter.PromptPassword(confirmMsg)
	if err != nil {
		return
	}
	if password != pass2 {
		err = errors.New("passwords do not match")
	}
	return
}
