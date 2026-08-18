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

func TestRenderMentionsSingle(t *testing.T) {
	text := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for details"
	resolved := RenderMentions(text, func(id string) string {
		if id == "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
			return "Login validation"
		}
		return ""
	})
	expected := "See @Login validation for details"
	if resolved != expected {
		t.Fatalf("RenderMentions = %q, want %q", resolved, expected)
	}
}

func TestRenderMentionsMultiple(t *testing.T) {
	text := "Related to @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 and @01ARZ9Y6Z7A8B9C0D1E2F3G4H5"
	resolved := RenderMentions(text, func(id string) string {
		switch id {
		case "01ARZ8X5Y6Z7A8B9C0D1E2F3G4":
			return "Login validation"
		case "01ARZ9Y6Z7A8B9C0D1E2F3G4H5":
			return "Session timeout"
		}
		return ""
	})
	expected := "Related to @Login validation and @Session timeout"
	if resolved != expected {
		t.Fatalf("RenderMentions = %q, want %q", resolved, expected)
	}
}

func TestRenderMentionsDeletedTask(t *testing.T) {
	text := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for details"
	resolved := RenderMentions(text, func(id string) string {
		return "" // task doesn't exist
	})
	expected := "See [deleted task] for details"
	if resolved != expected {
		t.Fatalf("RenderMentions = %q, want %q", resolved, expected)
	}
}

func TestRenderMentionsNoMentions(t *testing.T) {
	text := "plain text with no mentions"
	resolved := RenderMentions(text, func(id string) string {
		return "should not be called"
	})
	if resolved != text {
		t.Fatalf("RenderMentions should return original text when no mentions, got %q", resolved)
	}
}

func TestRenderMentionsEmptyText(t *testing.T) {
	resolved := RenderMentions("", func(id string) string {
		return "should not be called"
	})
	if resolved != "" {
		t.Fatalf("RenderMentions empty text = %q, want empty", resolved)
	}
}

func TestBuildMentionMetadataSingle(t *testing.T) {
	text := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for details"
	metadata := BuildMentionMetadata(text, func(id string) string {
		if id == "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
			return "Login validation"
		}
		return ""
	})
	if len(metadata) != 1 {
		t.Fatalf("expected 1 metadata entry, got %d", len(metadata))
	}
	if metadata[0].ID != "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
		t.Fatalf("ID = %q", metadata[0].ID)
	}
	if metadata[0].Title == nil || *metadata[0].Title != "Login validation" {
		t.Fatalf("Title = %v, want \"Login validation\"", metadata[0].Title)
	}
	if metadata[0].Start != 4 {
		t.Fatalf("Start = %d, want 4", metadata[0].Start)
	}
	if metadata[0].End != 31 {
		t.Fatalf("End = %d, want 31", metadata[0].End)
	}
	if metadata[0].Deleted {
		t.Fatalf("Deleted = true, want false")
	}
}

func TestBuildMentionMetadataMultiple(t *testing.T) {
	text := "Related to @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 and @01ARZ9Y6Z7A8B9C0D1E2F3G4H5"
	metadata := BuildMentionMetadata(text, func(id string) string {
		switch id {
		case "01ARZ8X5Y6Z7A8B9C0D1E2F3G4":
			return "Login validation"
		case "01ARZ9Y6Z7A8B9C0D1E2F3G4H5":
			return "Session timeout"
		}
		return ""
	})
	if len(metadata) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(metadata))
	}
	if metadata[0].ID != "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
		t.Fatalf("first ID = %q", metadata[0].ID)
	}
	if metadata[1].ID != "01ARZ9Y6Z7A8B9C0D1E2F3G4H5" {
		t.Fatalf("second ID = %q", metadata[1].ID)
	}
	if metadata[0].Title == nil || *metadata[0].Title != "Login validation" {
		t.Fatalf("first Title = %v", metadata[0].Title)
	}
	if metadata[1].Title == nil || *metadata[1].Title != "Session timeout" {
		t.Fatalf("second Title = %v", metadata[1].Title)
	}
}

func TestBuildMentionMetadataDeletedTask(t *testing.T) {
	text := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for details"
	metadata := BuildMentionMetadata(text, func(id string) string {
		return "" // task doesn't exist
	})
	if len(metadata) != 1 {
		t.Fatalf("expected 1 metadata entry, got %d", len(metadata))
	}
	if metadata[0].ID != "01ARZ8X5Y6Z7A8B9C0D1E2F3G4" {
		t.Fatalf("ID = %q", metadata[0].ID)
	}
	if metadata[0].Title != nil {
		t.Fatalf("Title = %v, want nil", metadata[0].Title)
	}
	if !metadata[0].Deleted {
		t.Fatalf("Deleted = false, want true")
	}
	if metadata[0].Start != 4 || metadata[0].End != 31 {
		t.Fatalf("Start/End = %d/%d, want 4/31", metadata[0].Start, metadata[0].End)
	}
}

func TestBuildMentionMetadataNoMentions(t *testing.T) {
	metadata := BuildMentionMetadata("plain text", func(id string) string {
		return "should not be called"
	})
	if metadata != nil {
		t.Fatalf("expected nil for text without mentions, got %v", metadata)
	}
}

func TestBuildMentionMetadataEmptyText(t *testing.T) {
	metadata := BuildMentionMetadata("", func(id string) string {
		return "should not be called"
	})
	if metadata != nil {
		t.Fatalf("expected nil for empty text, got %v", metadata)
	}
}
