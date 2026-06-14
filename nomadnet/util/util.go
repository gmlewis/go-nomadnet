// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Package util provides text sanitization utilities for NomadNet.
//
// These functions strip modifiers, sanitize display names, and
// remove or unescape Micron markup from text strings.
package util

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var invalidRendering = []string{"🕵️", "☝"}

// stripBlocksRe matches emoji, symbols, dingbats, variation selectors,
// and other decorative Unicode blocks that should be stripped.
var stripBlocksRe = regexp.MustCompile(
	`[\p{So}\p{Sk}` +
		`\x{1F600}-\x{1F64F}` + // Emoticons
		`\x{1F300}-\x{1F5FF}` + // Misc Symbols & Pictographs
		`\x{1F680}-\x{1F6FF}` + // Transport & Map Symbols
		`\x{1F700}-\x{1F77F}` + // Alchemical Symbols
		`\x{1F780}-\x{1F7FF}` + // Geometric Shapes Extended
		`\x{1F800}-\x{1F8FF}` + // Supplemental Arrows-C
		`\x{1F900}-\x{1F9FF}` + // Supplemental Symbols & Pictographs
		`\x{1FA00}-\x{1FA6F}` + // Chess Symbols
		`\x{1FA70}-\x{1FAFF}` + // Symbols & Pictographs Extended-A
		`\x{1F1E0}-\x{1F1FF}` + // Flags (regional indicators)
		`\x{2600}-\x{26FF}` + // Misc Symbols
		`\x{2700}-\x{27BF}` + // Dingbats
		`\x{FE00}-\x{FE0F}` + // Variation Selectors
		`\x{E0100}-\x{E01EF}` + // Variation Selectors Supplement
		`\x{1F3FB}-\x{1F3FF}` + // Emoji modifiers (skin tones)
		`]+`,
)

// stripControlRe matches control characters, zero-width characters,
// and other formatting codepoints that should be stripped.
var stripControlRe = regexp.MustCompile(
	`[\x00-\x08` + // C0 controls (NUL-BS)
		`\x0B\x0C` + // VT, FF
		`\x0E-\x1F` + // C0 controls (SO-US)
		`\x7F-\x9F` + // DEL and C1 controls
		`\x{200B}-\x{200F}` + // Zero-width chars, LRM, RLM, etc.
		`\x{202A}-\x{202E}` + // Bidi embedding controls
		`\x{2060}-\x{206F}` + // Format chars (word joiner, etc.)
		`\x{FEFF}` + // BOM / Zero Width NBSP
		`\x{FFF0}-\x{FFF8}` + // Specials
		`]+`,
)

// stripPrivateRe matches surrogates, private use areas,
// and other non-standard Unicode ranges.
var stripPrivateRe = regexp.MustCompile(
	`[\x{E000}-\x{F8FF}` + // Private Use Area
		`\x{F900}-\x{FAFF}` + // CJK Compatibility Ideographs
		`\x{FE10}-\x{FE1F}` + // Vertical Forms
		`\x{FE20}-\x{FE2F}` + // Combining Half Marks
		`\x{F0000}-\x{FFFFF}` + // Supplementary Private Use Area-A
		`\x{100000}-\x{10FFFF}` + // Supplementary Private Use Area-B
		`]+`,
)

// categoryPrefix returns the first letter of a Unicode category for the
// given rune (L, N, P, S, M, Z, C, or I for unknown).
func categoryPrefix(r rune) string {
	switch {
	case unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Lo, unicode.Lm):
		return "L"
	case unicode.In(r, unicode.Nd, unicode.Nl, unicode.No):
		return "N"
	case unicode.In(r, unicode.Pc, unicode.Pd, unicode.Ps, unicode.Pe, unicode.Pi, unicode.Pf, unicode.Po):
		return "P"
	case unicode.In(r, unicode.Sm, unicode.Sc, unicode.Sk, unicode.So):
		return "S"
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc):
		return "M"
	case unicode.In(r, unicode.Zl, unicode.Zp, unicode.Zs):
		return "Z"
	case unicode.In(r, unicode.Cc, unicode.Cf, unicode.Cs, unicode.Co, unicode.Cn):
		return "C"
	default:
		return "I"
	}
}

// processCharacters strips modifier and formatting characters from text
// while keeping letters, numbers, punctuation, and symbols.
func processCharacters(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		prefix := categoryPrefix(r)
		switch {
		case prefix == "L", prefix == "N", prefix == "P", prefix == "S":
			b.WriteRune(r)
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc),
			unicode.In(r, unicode.Cf),
			r == '\u200d',
			r == '\u200c':
			// Skip modifier/formatting chars
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// StripModifiers removes modifier characters, emoji variation selectors,
// zero-width characters, and other Unicode noise from text.
// Returns nil if text is nil.
func StripModifiers(text *string) *string {
	if text == nil {
		return nil
	}

	t := *text
	for _, bad := range invalidRendering {
		t = strings.ReplaceAll(t, bad, " ")
	}

	stripped := processCharacters(t)
	stripped = regexp.MustCompile(`[\x{FE00}-\x{FE0F}]`).ReplaceAllString(stripped, "")
	stripped = regexp.MustCompile(`[\x{E0100}-\x{E01EF}]`).ReplaceAllString(stripped, "")
	stripped = regexp.MustCompile(`[\x{1F3FB}-\x{1F3FF}]`).ReplaceAllString(stripped, "")
	stripped = regexp.MustCompile(`[\x{200D}\x{200C}]`).ReplaceAllString(stripped, "")
	stripped = regexp.MustCompile(`\r\n?`).ReplaceAllString(stripped, "\n")

	stripped = strings.TrimSpace(stripped)
	stripped = strings.ReplaceAll(stripped, "\x00", "")

	return &stripped
}

// SanitizeName normalizes and cleans a display name by stripping
// emoji, symbols, control characters, and collapsing whitespace.
// Returns nil if name is nil.
func SanitizeName(name *string) *string {
	if name == nil {
		return nil
	}

	n := norm.NFKC.String(*name)

	// Category-based filtering
	var b strings.Builder
	b.Grow(len(n))
	for _, r := range n {
		switch {
		case unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Lo),
			unicode.In(r, unicode.Nd, unicode.Nl, unicode.No),
			unicode.In(r, unicode.Pc, unicode.Pd, unicode.Ps, unicode.Pe, unicode.Pi, unicode.Pf, unicode.Po):
			b.WriteRune(r)
		case unicode.In(r, unicode.Zs):
			b.WriteRune(' ')
		case unicode.In(r, unicode.Zl, unicode.Zp):
			b.WriteRune(' ')
		case unicode.In(r, unicode.Mc):
			b.WriteRune(r)
		case unicode.In(r, unicode.Lm):
			b.WriteRune(r)
		default:
			// Strip Mn, Me, C*, S*, etc.
		}
	}

	result := b.String()
	result = stripBlocksRe.ReplaceAllString(result, "")
	result = stripControlRe.ReplaceAllString(result, "")
	result = stripPrivateRe.ReplaceAllString(result, "")

	// Collapse whitespace
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	return &result
}

// stripMicronRe removes Micron markup color/format codes.
var stripMicronRe = regexp.MustCompile("`[FB][0-9a-fA-F]{3}")
var stripMicronFTRe = regexp.MustCompile("`[FB]T[0-9a-fA-F]{6}")
var stripMicronTagRe = regexp.MustCompile("`[!*_=]")
var stripMicronFBRe = regexp.MustCompile("`f`b")
var stripMicronFRe = regexp.MustCompile("`f")
var stripMicronBRe = regexp.MustCompile("`b")
var stripMicronLtRe = regexp.MustCompile("`<")
var stripMicronGtRe = regexp.MustCompile("`>")
var stripMicronBraceRe = regexp.MustCompile("`\\{")

// StripMicron removes Micron markup tags from text, leaving only
// the visible content.
func StripMicron(text string) string {
	text = stripMicronRe.ReplaceAllString(text, "")
	text = stripMicronFTRe.ReplaceAllString(text, "")
	text = stripMicronTagRe.ReplaceAllString(text, "")
	text = stripMicronFBRe.ReplaceAllString(text, "")
	text = stripMicronFRe.ReplaceAllString(text, "")
	text = stripMicronBRe.ReplaceAllString(text, "")
	text = stripMicronLtRe.ReplaceAllString(text, "")
	text = stripMicronGtRe.ReplaceAllString(text, "")
	text = stripMicronBraceRe.ReplaceAllString(text, "")
	return text
}

// stripEscapedMicronRe removes escaped Micron markup (using ¦ instead of `).
var stripEscapedMicronRe = regexp.MustCompile(`\x{A6}[FB][0-9a-fA-F]{3}`)
var stripEscapedMicronFTRe = regexp.MustCompile(`\x{A6}[FB]T[0-9a-fA-F]{6}`)
var stripEscapedMicronTagRe = regexp.MustCompile(`\x{A6}[!*_=]`)
var stripEscapedMicronFBRe = regexp.MustCompile(`\x{A6}f\x{60}b`)
var stripEscapedMicronFRe = regexp.MustCompile(`\x{A6}f`)
var stripEscapedMicronBRe = regexp.MustCompile(`\x{A6}b`)
var stripEscapedMicronLtRe = regexp.MustCompile(`\x{A6}<`)
var stripEscapedMicronGtRe = regexp.MustCompile(`\x{A6}>`)
var stripEscapedMicronBraceRe = regexp.MustCompile(`\x{A6}\{`)

// StripEscapedMicron removes escaped Micron markup (using ¦ prefix)
// from text.
func StripEscapedMicron(text string) string {
	text = stripEscapedMicronRe.ReplaceAllString(text, "")
	text = stripEscapedMicronFTRe.ReplaceAllString(text, "")
	text = stripEscapedMicronTagRe.ReplaceAllString(text, "")
	text = stripEscapedMicronFBRe.ReplaceAllString(text, "")
	text = stripEscapedMicronFRe.ReplaceAllString(text, "")
	text = stripEscapedMicronBRe.ReplaceAllString(text, "")
	text = stripEscapedMicronLtRe.ReplaceAllString(text, "")
	text = stripEscapedMicronGtRe.ReplaceAllString(text, "")
	text = stripEscapedMicronBraceRe.ReplaceAllString(text, "")
	return text
}

// unescapeMicronRe converts escaped Micron markup (¦) back to
// regular Micron markup (`).
var unescapeMicronRe = regexp.MustCompile(`\x{A6}([FB][0-9a-fA-F]{3})`)
var unescapeMicronFTRe = regexp.MustCompile(`\x{A6}([FB]T[0-9a-fA-F]{6})`)
var unescapeMicronTagRe = regexp.MustCompile(`\x{A6}([!*_=])`)
var unescapeMicronFBRe = regexp.MustCompile(`\x{A6}(f\x{60}b)`)
var unescapeMicronFRe = regexp.MustCompile(`\x{A6}(f)`)
var unescapeMicronBRe = regexp.MustCompile(`\x{A6}(b)`)
var unescapeMicronLtRe = regexp.MustCompile(`\x{A6}(<)`)
var unescapeMicronGtRe = regexp.MustCompile(`\x{A6}(>)`)
var unescapeMicronBraceRe = regexp.MustCompile(`\x{A6}(\{)`)

// UnescapeMicron converts escaped Micron markup (using ¦ prefix)
// back to regular Micron markup (using ` prefix).
func UnescapeMicron(text string) string {
	text = unescapeMicronRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronFTRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronTagRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronFBRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronFRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronBRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronLtRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronGtRe.ReplaceAllString(text, "`$1")
	text = unescapeMicronBraceRe.ReplaceAllString(text, "`$1")
	return text
}

// stripNonFormattingTagsRe removes non-formatting Micron tags
// (alignment, heading, block) from text.
var stripNonFormattingTagsLtRe = regexp.MustCompile("`<")
var stripNonFormattingTagsGtRe = regexp.MustCompile("`>")
var stripNonFormattingTagsBraceRe = regexp.MustCompile("`\\{")
var stripNonFormattingTagsRRe = regexp.MustCompile("`r")
var stripNonFormattingTagsCRe = regexp.MustCompile("`c")
var stripNonFormattingTagsLRe = regexp.MustCompile("`l")

// StripNonFormattingTags removes non-formatting Micron tags from text
// while preserving formatting tags like bold, italic, underline.
func StripNonFormattingTags(text string) string {
	text = stripNonFormattingTagsLtRe.ReplaceAllString(text, "")
	text = stripNonFormattingTagsGtRe.ReplaceAllString(text, "")
	text = stripNonFormattingTagsBraceRe.ReplaceAllString(text, "")
	text = stripNonFormattingTagsRRe.ReplaceAllString(text, "")
	text = stripNonFormattingTagsCRe.ReplaceAllString(text, "")
	text = stripNonFormattingTagsLRe.ReplaceAllString(text, "")
	return text
}
