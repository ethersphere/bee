package terminal

import (
	"io"
)

// WithPromptOut sets the out file to write the prompt to.
func WithPromptOut(out io.Writer) PasswordPrompterOption {
	return func(fpp *WriterPasswordPrompter) {
		fpp.out = out
	}
}

// WithPromptPasswordReader sets the password reader.
func WithPromptPasswordReader(pwReader PasswordReader) PasswordPrompterOption {
	return func(fpp *WriterPasswordPrompter) {
		fpp.pwReader = pwReader
	}
}

// WithConfirmOut sets the out file to write the prompt to.
func WithConfirmOut(out io.Writer) PasswordConfirmerOption {
	return func(fpp *WriterPasswordConfirmer) {
		fpp.out = out
	}
}

// WithConfirmPasswordPrompter sets the password reader.
func WithConfirmPasswordPrompter(pwPrompter PasswordPrompter) PasswordConfirmerOption {
	return func(fpc *WriterPasswordConfirmer) {
		fpc.pwPrompter = pwPrompter
	}
}
