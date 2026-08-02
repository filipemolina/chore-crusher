// Package mcpserver exposes Chore Crusher's store operations as MCP tools,
// so an agent can call them through the Model Context Protocol instead of
// shelling out to the CLI. It is a thin adapter over src/store: it does not
// import src/cli or src/model, keeping the MCP wrapper a sibling front end
// rather than a layer on either existing one (docs/DESIGN.md §1, §10).
package mcpserver
