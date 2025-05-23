package main

import "fmt"

func main() {
	fmt.Println("Hello, GO!")

	num := 7

	if num > 5 { // 'num' is scoped to the if/else block
		fmt.Println("Num is greater than 5")
	} else {
		fmt.Println("Num is 5 or less")
	}
}
