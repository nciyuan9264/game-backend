package main

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func main1() {
	password := "123456"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	fmt.Println(string(hash))
}
