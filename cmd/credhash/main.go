package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	secret, err := readSecret()
	if err != nil {
		fmt.Fprintf(os.Stderr, "read secret: %v\n", err)
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash secret: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(hash))
}

func readSecret() (string, error) {
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		return os.Args[1], nil
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return "", fmt.Errorf("provide secret as arg or stdin")
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	secret := strings.TrimSpace(line)
	if secret == "" {
		return "", fmt.Errorf("secret is empty")
	}
	return secret, nil
}
