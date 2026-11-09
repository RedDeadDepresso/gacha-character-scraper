package scrapers

// Manual is a game whose data/<game>.json is maintained by hand. It scrapes
// nothing: the saved characters are returned as they are, so that main still
// applies the Bangumi score, collab flag and birthday to them and saves the
// file again.
type Manual struct{ Game string }

func (m Manual) Name() string { return m.Game }

func (Manual) Scrape(existing []Character) ([]Character, error) { return existing, nil }