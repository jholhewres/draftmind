// Package curated embeds the curated data files the sync loads (ADR-0011).
package curated

import _ "embed"

// ItemCounters is item_counters.json: which items answer which tags, and
// the tags heroes bring, by curated hero slug or by dotaconstants role.
//
//go:embed item_counters.json
var ItemCounters []byte
