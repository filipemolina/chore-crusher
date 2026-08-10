package apptypes

import (
	"sort"
	"strings"
)

// SortMode defines how tasks are ordered in the tree view.
type SortMode int

const (
	SortManual       SortMode = iota // default, respects position
	SortPriority                     // high → medium → low → none
	SortCreated                      // newest first
	SortUpdated                      // most recent first
	SortAlphabetical                 // A-Z by title
)

// String returns the display name for the sort mode.
func (s SortMode) String() string {
	switch s {
	case SortPriority:
		return "priority"
	case SortCreated:
		return "created"
	case SortUpdated:
		return "updated"
	case SortAlphabetical:
		return "alpha"
	default:
		return "manual"
	}
}

// Next cycles to the next sort mode, wrapping back to Manual after Alphabetical.
func (s SortMode) Next() SortMode {
	return (s + 1) % (SortAlphabetical + 1)
}

// SortTasks returns a sorted copy of the tasks based on the specified mode.
// The original slice is not modified.
func SortTasks(tasks []Task, mode SortMode) []Task {
	sorted := make([]Task, len(tasks))
	copy(sorted, tasks)

	switch mode {
	case SortManual:
		// No sorting, return as-is
		return sorted
	case SortPriority:
		sort.SliceStable(sorted, func(i, j int) bool {
			return priorityRank(sorted[i].Priority) < priorityRank(sorted[j].Priority)
		})
	case SortCreated:
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].CreatedAt > sorted[j].CreatedAt
		})
	case SortUpdated:
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].UpdatedAt > sorted[j].UpdatedAt
		})
	case SortAlphabetical:
		sort.SliceStable(sorted, func(i, j int) bool {
			return strings.ToLower(sorted[i].Title) < strings.ToLower(sorted[j].Title)
		})
	}

	return sorted
}

// priorityRank returns a numeric rank for priority sorting.
// Lower numbers = higher priority (sorted first).
func priorityRank(p Priority) int {
	switch p {
	case PriorityHigh:
		return 0
	case PriorityMedium:
		return 1
	case PriorityLow:
		return 2
	default:
		return 3
	}
}
