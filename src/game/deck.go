package game

// A deck of playing cards
type Deck struct {
	Cards [52]Card
}

// Create a new 52 card deck containing all cards
func NewDeck() Deck {
	deck := Deck{}
	idx := 0

	for _, suit := range AllCardSuits {
		for _, value := range AllCardValues {
			deck.Cards[idx] = NewCard(value, suit)
			idx++
		}
	}

	return deck
}
