package mock

import (
	"errors"

	"github.com/ethersphere/bee/cmd/internal/terminal"
)

var (
	_ terminal.PasswordReader   = (*PasswordReader)(nil)
	_ terminal.PasswordPrompter = (*PasswordPrompter)(nil)
)

// PasswordReader mocks the PasswordReader interface.
type PasswordReader struct {
	password string
	err      bool
}

// NewMockPasswordReader creates the mock PasswordReader.
func NewMockPasswordReader(password string, returnErr bool) *PasswordReader {
	return &PasswordReader{password, returnErr}
}

// ReadPassword implements the PasswordReader interface.
func (pr *PasswordReader) ReadPassword() (password string, err error) {
	if pr.err {
		err = errors.New("failed to read password")
		return
	}
	password = pr.password
	return
}

// PasswordPrompter mocks the PasswordPrompter interface.
type PasswordPrompter struct {
	pass1         string
	pass2         string
	useSecondPass bool
	err           bool
	cntUntilErr   int
}

// NewMockPasswordPrompter creates the mock PasswordPrompter.
func NewMockPasswordPrompter(pass1, pass2 string, err bool, errorFromCnt int) *PasswordPrompter {
	return &PasswordPrompter{pass1, pass2, false, err, errorFromCnt}
}

// PromptPassword implements the PasswordPrompter interface.
func (pp *PasswordPrompter) PromptPassword(msg string) (password string, err error) {
	if pp.useSecondPass {
		password = pp.pass2
	} else {
		password = pp.pass1
	}
	pp.useSecondPass = !pp.useSecondPass

	if pp.err {
		if pp.cntUntilErr > 0 {
			pp.cntUntilErr--
		} else {
			err = errors.New("failed to read password")
		}

	}

	return
}
