package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

// printResult writes a subcommand's payload to stdout in exactly one of two
// shapes (docs/DESIGN.md §9): JSON mode emits exactly one JSON value; human
// mode runs humanFn. Every subcommand's output ends here or in printError —
// never a bespoke fmt.Println in a RunE — which is what keeps the "exactly
// one JSON value on stdout in --json mode" contract mechanical rather than a
// rule each command has to remember.
func printResult(jsonMode bool, humanFn func(), payload any) {
	if !jsonMode {
		humanFn()
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		// Every payload here is json-marshalable by construction; if one
		// ever isn't, still emit one JSON value rather than nothing.
		b = []byte(`{"error":"encode result: ` + err.Error() + `"}`)
	}
	fmt.Println(string(b))
}

// printError reports a failure per docs/DESIGN.md §9 and returns the exit
// code it implies (1): in JSON mode {"error": ...} goes to stdout — the one
// stream a --json caller reads, so it never checks two — and in human mode
// "crush: ..." goes to stderr. The returned code and the one Execute
// derives from the wrapped error always agree; the int exists so the §9
// contract has one obvious place to read "domain failure means 1".
func printError(jsonMode bool, err error) int {
	if jsonMode {
		b, _ := json.Marshal(struct {
			Error string `json:"error"`
		}{err.Error()})
		fmt.Println(string(b))
		return 1
	}
	fmt.Fprintln(os.Stderr, "crush:", err)
	return 1
}
