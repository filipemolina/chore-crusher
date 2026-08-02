package addinput

import "testing"

// TestNextOffset tests the nextOffset function against the spec table from
// docs/plans/phase-5-add-input.md §2.
func TestNextOffset(t *testing.T) {
	tests := []struct {
		name           string
		selectedDepth  int
		currentOffset  int
		key            string
		expectedOffset int
	}{
		{"root at default, tab", 0, 0, "tab", 1},
		{"root at default, shift+tab", 0, 0, "shift+tab", 0},
		{"root at +1, tab", 0, 1, "tab", 1},
		{"root at +1, shift+tab", 0, 1, "shift+tab", 0},
		{"depth 2 at default, shift+tab", 2, 0, "shift+tab", -1},
		{"depth 2 at -1, shift+tab", 2, -1, "shift+tab", -1},
		{"depth 2 at -1, tab", 2, -1, "tab", 0},
		{"depth 2 at default, tab", 2, 0, "tab", 1},
		{"depth 2 at +1, tab", 2, 1, "tab", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextOffset(tt.currentOffset, tt.key, tt.selectedDepth)
			if got != tt.expectedOffset {
				t.Errorf("nextOffset(%d, %q, %d) = %d, want %d",
					tt.currentOffset, tt.key, tt.selectedDepth, got, tt.expectedOffset)
			}
		})
	}
}
