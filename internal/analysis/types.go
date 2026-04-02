package analysis

type VocabEntry struct {
	Word       string `json:"word"`
	Definition string `json:"definition"`
	Sentence   string `json:"sentence"`
}

type Quote struct {
	Text string `json:"text"`
	Note string `json:"note"`
}

type Response struct {
	Title        string       `json:"title"`
	Summary      string       `json:"summary"`
	Vocabulary   []VocabEntry `json:"vocabulary"`
	Quotes       []Quote      `json:"quotes"`
	Missed       string       `json:"missed"`
	Connections  string       `json:"connections"`
	ImageQuality string       `json:"image_quality"`
}
