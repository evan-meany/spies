package main

import (
	"fmt"
	"spies/src/game"
)

func main() {
	deck := game.NewDeck()
	fmt.Printf("deck: %v\n", deck)
}
