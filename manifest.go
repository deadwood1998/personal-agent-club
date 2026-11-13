// Package pac holds what the program embeds from the repository root: the Persona extension manifest.
package pac

import (
	_ "embed"
	"encoding/json"
)

// Manifest is persona-extension.json.
//
//go:embed persona-extension.json
var Manifest []byte

// Version is the manifest's version: the program's own unless a build sets another. The hub expects initialize to
// answer the version of the package it installed.
func Version() string {
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(Manifest, &m)
	return m.Version
}
