package game

// A playing card (e.g. a Jack of Spades)
type Card struct {
	// Card value (e.g. a King)
	Value CardValue

	// Card Suit (e.g. Spades)
	Suit CardSuit

	// Card point value (e.g. Aces are worth 3 points in Spies)
	// Points int
}

// Create a new card
func NewCard(v CardValue, s CardSuit) Card {
	return Card{Value: v, Suit: s}
}

// Stringify the card
func (c Card) String() string {
	return c.Value.String() + " of " + c.Suit.String()
}
