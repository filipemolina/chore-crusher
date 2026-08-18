package mentions

import (
	"regexp"
	"strings"
)

// Mention represents a @task-id reference found in text.
// Start and End are byte offsets into the original text.
type Mention struct {
	ID    string
	Start int
	End   int
}

// MentionMetadata is the JSON representation of a mention for CLI output.
// Deleted tasks have Title set to null and Deleted set to true.
type MentionMetadata struct {
	ID      string  `json:"id"`
	Title   *string `json:"title"`
	Start   int     `json:"start"`
	End     int     `json:"end"`
	Deleted bool    `json:"deleted,omitempty"`
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

// RenderMentions replaces @<ULID> patterns in text with their resolved titles.
// The resolver function takes a task ID and returns the task's title, or an
// empty string if the task doesn't exist (deleted). Deleted tasks render as
// "[deleted task]". This is a pure function with no side effects.
func RenderMentions(text string, resolver func(string) string) string {
	mentions := ParseMentions(text)
	if mentions == nil {
		return text
	}

	var result strings.Builder
	lastEnd := 0
	for _, m := range mentions {
		// Write text before the mention
		result.WriteString(text[lastEnd:m.Start])

		// Resolve the mention
		title := resolver(m.ID)
		if title == "" {
			result.WriteString("[deleted task]")
		} else {
			result.WriteString("@")
			result.WriteString(title)
		}

		lastEnd = m.End
	}

	// Write remaining text after the last mention
	result.WriteString(text[lastEnd:])

	return result.String()
}

// BuildMentionMetadata creates JSON-serializable mention metadata from text.
// The resolver returns the task title, or empty string if the task doesn't exist.
// Deleted tasks get Title=null and Deleted=true in the metadata.
func BuildMentionMetadata(text string, resolver func(string) string) []MentionMetadata {
	mentions := ParseMentions(text)
	if mentions == nil {
		return nil
	}

	metadata := make([]MentionMetadata, 0, len(mentions))
	for _, m := range mentions {
		title := resolver(m.ID)
		if title == "" {
			metadata = append(metadata, MentionMetadata{
				ID:      m.ID,
				Title:   nil,
				Start:   m.Start,
				End:     m.End,
				Deleted: true,
			})
		} else {
			metadata = append(metadata, MentionMetadata{
				ID:    m.ID,
				Title: &title,
				Start: m.Start,
				End:   m.End,
			})
		}
	}
	return metadata
}
