package mentions

import (
	"regexp"
)

// Mention represents a @task-id reference found in text.
// Start and End are byte offsets into the original text.
type Mention struct {
	ID    string
	Start int
	End   int
}

// mentionRE matches @ followed by a 26-character ULID.
// ULIDs are 26 characters: 10 chars timestamp + 16 chars randomness, base32 encoded (Crockford's alphabet).
var mentionRE = regexp.MustCompile(`@([0-9A-HJKMNP-TV-Z]{26})`)

// ParseMentions extracts all @<ULID> mentions from the given text.
// It returns a slice of Mention structs with the task ID and byte offsets.
// Non-ULID @text patterns (e.g., @user, @abc) are ignored.
func ParseMentions(text string) []Mention {
	matches := mentionRE.FindAllStringSubmatchIndex(text, -1)
	if matches == nil {
		return nil
	}
	mentions := make([]Mention, 0, len(matches))
	for _, m := range matches {
		// m[0], m[1] = full match (@ULID)
		// m[2], m[3] = capture group 1 (ULID only)
		mentions = append(mentions, Mention{
			ID:    text[m[2]:m[3]],
			Start: m[0],
			End:   m[1],
		})
	}
	return mentions
}
