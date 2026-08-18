package mentions

import (
	"testing"
)

func TestParseMentionsValid(t *testing.T) {
	text := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for details"
	mentions := ParseMentions(text)
	if len(mentions) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(mentions))
	}
	if mentions[0].ID != "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
		t.Fatalf("mention ID = %q, want %q", mentions[0].ID, "01ARZ8X5Y6Z7A8B9C0D1E2F3G4")
	}
	if mentions[0].Start != 4 {
		t.Fatalf("mention Start = %d, want 4", mentions[0].Start)
	}
	// @ + 26 char ULID = 27 chars total
	if mentions[0].End != 31 {
		t.Fatalf("mention End = %d, want 31", mentions[0].End)
	}
}

func TestParseMentionsMultiple(t *testing.T) {
	text := "Related to @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 and @01ARZ9Y6Z7A8B9C0D1E2F3G4H5"
	mentions := ParseMentions(text)
	if len(mentions) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(mentions))
	}
	if mentions[0].ID != "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
		t.Fatalf("first mention ID = %q", mentions[0].ID)
	}
	if mentions[1].ID != "01ARZ9Y6Z7A8B9C0D1E2F3G4H5" {
		t.Fatalf("second mention ID = %q", mentions[1].ID)
	}
}

func TestParseMentionsAtStart(t *testing.T) {
	text := "@01ARZ8X5Y6Z7A8B9C0D1E2F3G4 is the first task"
	mentions := ParseMentions(text)
	if len(mentions) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(mentions))
	}
	if mentions[0].Start != 0 {
		t.Fatalf("mention Start = %d, want 0", mentions[0].Start)
	}
}

func TestParseMentionsAtEnd(t *testing.T) {
	text := "The task is @01ARZ8X5Y6Z7A8B9C0D1E2F3G4"
	mentions := ParseMentions(text)
	if len(mentions) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(mentions))
	}
	if mentions[0].End != len(text) {
		t.Fatalf("mention End = %d, want %d", mentions[0].End, len(text))
	}
}

func TestParseMentionsConsecutive(t *testing.T) {
	text := "@01ARZ8X5Y6Z7A8B9C0D1E2F3G4@01ARZ9Y6Z7A8B9C0D1E2F3G4H5"
	mentions := ParseMentions(text)
	if len(mentions) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(mentions))
	}
	if mentions[0].End != mentions[1].Start {
		t.Fatalf("consecutive mentions should touch: first End=%d, second Start=%d", mentions[0].End, mentions[1].Start)
	}
}

func TestParseMentionsNonULIDIgnored(t *testing.T) {
	text := "Hello @user and @abc123 and @01ARZ8X5Y6Z7A8B9C0D1E2F3G4"
	mentions := ParseMentions(text)
	if len(mentions) != 1 {
		t.Fatalf("expected 1 mention (only ULID), got %d", len(mentions))
	}
	if mentions[0].ID != "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
		t.Fatalf("mention ID = %q", mentions[0].ID)
	}
}

func TestParseMentionsEmptyText(t *testing.T) {
	mentions := ParseMentions("")
	if mentions != nil {
		t.Fatalf("expected nil for empty text, got %v", mentions)
	}
}

func TestParseMentionsNoMentions(t *testing.T) {
	mentions := ParseMentions("plain text with no mentions")
	if mentions != nil {
		t.Fatalf("expected nil for text without mentions, got %v", mentions)
	}
}
