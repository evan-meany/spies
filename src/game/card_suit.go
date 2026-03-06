package game

// Card Suit type
type CardSuit int

// Possible suits
const (
	Hearts CardSuit = iota
	Diamonds
	Spades
	Clubs
)

// Card Suits
func (s CardSuit) String() string {
	switch s {
	case Hearts:
		return "Hearts"
	case Diamonds:
		return "Diamonds"
	case Spades:
		return "Spades"
	case Clubs:
		return "Clubs"
	default:
		return "Unknown"
	}
}

// All suits
var AllCardSuits = [4]CardSuit{
	Hearts,
	Diamonds,
	Spades,
	Clubs,
}
