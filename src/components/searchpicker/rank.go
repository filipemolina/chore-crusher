package searchpicker

import (
	"github.com/filipemolina/chore-crusher/src/store"
	"github.com/sahilm/fuzzy"
)

// rank runs store.SearchTasks across all lists and orders the results the
// same way the CLI's rankSearch does (docs/plans/phase-2-cli.md step 5): title
// matches first, ranked by fuzzy score, then candidates that matched only on
// notes in store order. Each result carries its list's name for context.
func rank(s *store.Store, query string) []Result {
	if query == "" {
		return nil
	}

	candidates, err := s.SearchTasks(query, nil)
	if err != nil {
		return nil // picker surface for a search failure; caller shows nothing
	}
	if len(candidates) == 0 {
		return nil
	}

	// Prepend the notes fallback so a notes-only hit still shows, but title
	// matches always rank above it regardless of fuzzy score.
	names := listNames(s)

	titles := make([]string, len(candidates))
	for i, c := range candidates {
		titles[i] = c.Title
	}

	matched := make([]bool, len(candidates))
	out := make([]Result, 0, len(candidates))
	for _, m := range fuzzy.Find(query, titles) {
		matched[m.Index] = true
		t := candidates[m.Index]
		out = append(out, Result{TaskID: t.ID, Title: t.Title, ListID: t.ListID, ListName: names[t.ListID]})
	}
	for i, c := range candidates {
		if matched[i] {
			continue
		}
		out = append(out, Result{TaskID: c.ID, Title: c.Title, ListID: c.ListID, ListName: names[c.ListID]})
	}
	return out
}

// listNames builds the list-id -> name map once per search so each result can
// carry its context without an N+1 lookups.
func listNames(s *store.Store) map[string]string {
	lists, err := s.ListLists()
	if err != nil {
		return map[string]string{}
	}
	names := make(map[string]string, len(lists))
	for _, l := range lists {
		names[l.ID] = l.Name
	}
	return names
}
