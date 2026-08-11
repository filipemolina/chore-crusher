package constants

// WORDMARK is the one-line badge rendered in the main menu bar.
var WORDMARK = "▌ Farol"

// APP_NAME is the lowercase, command-friendly name of the app.
var APP_NAME = "farol"

// DEFAULT_LIST_NAME is the name of the list the TUI creates for itself when
// the store has none — first run, or every list deleted. It is a name, not a
// placeholder: "New List" described the list's age rather than its contents,
// read as an unfinished setup step, and could only be corrected through R in
// the Lists panel, a panel the user may not have found and which is hidden
// below AUTO_SHOW_LISTS_MIN_WIDTH. "Inbox" matches what the MCP server already
// names an agent's own default list ("<tag>: Inbox", store.GetOrCreateAgentList).
//
// This is only the auto-created name. A list the user creates with n is named
// by the user in listnamemodal, and shares nothing with this.
var DEFAULT_LIST_NAME = "Inbox"
