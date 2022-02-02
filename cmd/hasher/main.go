package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	args := os.Args[1:]

	if len(args) != 1 {
		fmt.Println("usage:", "hasher", "your-plain-text-password")
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
