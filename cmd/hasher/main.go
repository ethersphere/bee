package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

const usage = `Usage: hasher [OPTION] {your-plain-text-password} [bcrypt-hash]
default (missing option)
	generate a bcrypt hash from the given plain text

--check
	check a bcrypt hash against a plain text

NOTE: the bcrypt hash argument has to be in single quotes or otherwise escaped`

func main() {
	isCheck := flag.Bool("check", false, "validate a hash against a plain text")
	flag.Parse()

	if *isCheck {
		doCheck()
		return
	}

	args := os.Args[1:]

	if len(args) != 1 {
		fmt.Println(usage)
		return
	}

	plainText := os.Args[1]

	hashed, err := bcrypt.GenerateFromPassword([]byte(plainText), bcrypt.DefaultCost)

	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to generate password hash")
		os.Exit(1)
	}

	fmt.Println(string(hashed))
}

func doCheck() {
	args := os.Args[2:]

	if len(args) != 2 {
		fmt.Println("usage:", "hasher", "--check", "your-plain-text-password", "'password-hash'")
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(args[1]), []byte(args[0]))

	if err != nil {
		fmt.Fprintln(os.Stderr, "password hash does not match provided plain text")
		os.Exit(1)
	}

	fmt.Println("OK: password hash matches provided plain text")
}
