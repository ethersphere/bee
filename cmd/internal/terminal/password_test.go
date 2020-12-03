package terminal_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/cmd/internal/terminal"
	"github.com/ethersphere/bee/cmd/internal/terminal/mock"
)

func TestFilePasswordReader(t *testing.T) {
	testCases := []struct {
		desc         string
		filePath     string
		fileContent  string
		wantPassword string
		wantErr      bool
	}{
		{
			desc:     "bad password file path",
			filePath: "notexistant",
			wantErr:  true,
		},
		{
			desc:         "read password from file",
			fileContent:  "testpassword",
			wantPassword: "testpassword",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.filePath == "" {
				tf, err := ioutil.TempFile(os.TempDir(), "bee-test")
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(tf.Name())
				tC.filePath = tf.Name()
				if _, err := tf.Write([]byte(tC.fileContent)); err != nil {
					t.Fatal(err)
				}
			}

			fpr := terminal.NewFilePasswordReader(tC.filePath)
			gotPassword, gotErr := fpr.ReadPassword()
			if gotErr != nil {
				if !tC.wantErr {
					t.Errorf("got error %v", gotErr)
				}
				return
			}
			if gotPassword != tC.wantPassword {
				t.Errorf("want: %s, got: %s", tC.wantPassword, gotPassword)
			}
		})
	}
}

func TestPromptPassword(t *testing.T) {
	testCases := []struct {
		desc          string
		msg           string
		errorInReader bool
		wantPassword  string
		wantError     bool
	}{
		{
			desc:          "error in reader",
			msg:           "Password: ",
			errorInReader: true,
			wantError:     true,
		},
		{
			desc:         "password read",
			msg:          "Password: ",
			wantPassword: "test",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			pwReader := mock.NewMockPasswordReader(
				tC.wantPassword,
				tC.errorInReader,
			)
			pp := terminal.NewPasswordPrompter(
				terminal.WithPromptOut(ioutil.Discard),
				terminal.WithPromptPasswordReader(pwReader),
			)
			gotPassword, gotError := pp.PromptPassword(tC.msg)
			if gotError != nil {
				if !tC.wantError {
					t.Errorf("got error: %v", gotError)
				}
				return
			}
			if tC.wantPassword != gotPassword {
				t.Errorf("want %s, got %s", tC.wantPassword, gotPassword)
			}
		})
	}
}

func TestPasswordConfirm(t *testing.T) {
	testCases := []struct {
		desc          string
		msg           string
		confirmMsg    string
		firstPass     string
		confirmedPass string
		errInPrompt   bool
		cntUntilErr   int
		wantPassword  string
		wantError     bool
	}{
		{
			desc:        "error in first prompt read",
			msg:         "Password: ",
			confirmMsg:  "Confirm password: ",
			errInPrompt: true,
			wantError:   true,
		},
		{
			desc:        "error in second prompt read",
			msg:         "Password: ",
			confirmMsg:  "Confirm password: ",
			errInPrompt: true,
			cntUntilErr: 1,
			wantError:   true,
		},
		{
			desc:          "passwords do not match",
			msg:           "Password: ",
			confirmMsg:    "Confirm password: ",
			firstPass:     "test",
			confirmedPass: "noooooooo",
			wantError:     true,
		},
		{
			desc:          "passwords match",
			msg:           "Password: ",
			confirmMsg:    "Confirm password: ",
			firstPass:     "test",
			confirmedPass: "test",
			wantPassword:  "test",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			pp := mock.NewMockPasswordPrompter(tC.firstPass, tC.confirmedPass, tC.errInPrompt, tC.cntUntilErr)
			pc := terminal.NewPasswordConfirmer(
				terminal.WithConfirmOut(ioutil.Discard),
				terminal.WithConfirmPasswordPrompter(pp),
			)
			gotPassword, gotError := pc.PromptConfirmPassword(tC.msg, tC.confirmMsg)
			if gotError != nil {
				if !tC.wantError {
					t.Errorf("got error: %v", gotError)
				}
				return
			}
			if tC.wantPassword != gotPassword {
				t.Errorf("want %s, got %s", tC.wantPassword, gotPassword)
			}
		})
	}
}
