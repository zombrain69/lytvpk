package parser

import (
	"strings"
	"testing"
)

func TestValidateAddonInfoContentAcceptsValveKeyValues(t *testing.T) {
	content := "\"AddonInfo\"\r\n{\r\n\taddonSteamAppID \"550\"\r\n\taddontitle \"Valid Mod\"\r\n\taddonDescription \"A complete description\"\r\n}\r\n"
	if err := validateAddonInfoContent(content); err != nil {
		t.Fatalf("valid addoninfo rejected: %v", err)
	}
}

func TestValidateAddonInfoContentRejectsUnclosedQuotedDescription(t *testing.T) {
	content := "\"AddonInfo\"\r\n{\r\n\taddonSteamAppID \"550\"\r\n\taddonDescription \"\r\nunfinished description\r\n}\r\n"
	err := validateAddonInfoContent(content)
	if err == nil {
		t.Fatal("malformed addoninfo was accepted")
	}
	if !strings.Contains(err.Error(), "KeyValues") {
		t.Fatalf("error = %q, want KeyValues parse failure", err)
	}
}
