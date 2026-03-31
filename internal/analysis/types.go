package analysis

type VocabEntry struct {
	Word             string `json:"word"`
	Definition       string `json:"definition_plain_english"`
	SentenceFromText string `json:"sentence_from_text"`
}

type QuoteEntry struct {
	Quote        string `json:"quote"`
	WhyItMatters string `json:"why_it_matters"`
}

type Connection struct {
	CurrentText string `json:"current_text"`
	ConnectsTo  string `json:"connects_to"`
	Insight     string `json:"insight"`
}

type Response struct {
	Summary      string       `json:"summary"`
	Vocabulary   []VocabEntry `json:"vocabulary"`
	Quotes       []QuoteEntry `json:"quotes"`
	Missed       []string     `json:"missed"`
	Connections  []Connection `json:"connections"`
	ImageQuality string       `json:"image_quality"`
}
