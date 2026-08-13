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

package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// debugPeekLog is a TEMPORARY diagnostic appended to /tmp/peek-debug.log to
// trace whether peekLink fires on the ICP Board landing-page link. Remove after
// diagnosing the test-gonomadnet-input-box link-follow failure.
func debugPeekLog(bd *BrowserDisplay, link *micron.LinkSpec) {
	f, err := os.OpenFile("/tmp/peek-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	cur := 0
	if bd.focusLine >= 0 && bd.focusLine < len(bd.lineCursors) {
		cur = bd.lineCursors[bd.focusLine]
	}
	plain := ""
	if bd.focusLine >= 0 && bd.focusLine < len(bd.currentLines) {
		plain = bd.linePlainText(bd.focusLine)
	}
	linkURL := ""
	if link != nil {
		linkURL = link.URL
	}
	_, _ = f.WriteString("focusLine=" + strconv.Itoa(bd.focusLine) + " cursor=" + strconv.Itoa(cur) + " plain=" + plain + " link=" + linkURL + "\n")
}

// debugInputLog is a TEMPORARY diagnostic appended to /tmp/peek-debug.log
// tracing every key the browser handleInput sees + whether bd.content has
// focus. Remove after diagnosing the link-follow failure.
func debugInputLog(bd *BrowserDisplay, event *tcell.EventKey) {
	f, err := os.OpenFile("/tmp/peek-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	name := event.Name()
	hasFocus := "no"
	if bd.content != nil && bd.content.HasFocus() {
		hasFocus = "yes"
	}
	_, _ = f.WriteString("INPUT key=" + name + " contentHasFocus=" + hasFocus + "\n")
}

// DebugAppKey is a TEMPORARY diagnostic appended to /tmp/peek-debug.log tracing
// every app-level key + the focused primitive type. Remove after diagnosing.
func DebugAppKey(app *tview.Application, event *tcell.EventKey) {
	f, err := os.OpenFile("/tmp/peek-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	name := event.Name()
	focus := "<nil>"
	if p := app.GetFocus(); p != nil {
		focus = fmt.Sprintf("%T", p)
	}
	_, _ = f.WriteString("APPKEY key=" + name + " focus=" + focus + "\n")
}

// renderedField is one rendered micron <field> span on a page line. The browser
// body is a single tview.TextView that cannot host child primitives, so a text
// field is exposed as an in-place ReadlineEdit overlay (drawn over the field's
// screen cells when its row is focused); a checkbox/radio field is toggled in
// place via Space/Enter. On a form-submit link, collectFields reads the live
// value from editor (text) or checkbox (checkbox/radio), mirroring Python
// recurse_down (Browser.py:232-268).
type renderedField struct {
	spec      *micron.FieldSpec
	editor    *ReadlineEdit   // non-nil for text fields (the overlay primitive)
	checkbox  *tview.Checkbox // non-nil for checkbox/radio fields
	startCol  int             // display width of text before this field on its line
	width     int             // field width in columns (defaultFieldWidth default)
	runeStart int             // rune offset of this field's span within its line
	runeEnd   int             // rune offset just past this field's span
}

// buildLineFields populates bd.lineFields from the rendered styled lines,
// mirroring the per-line widget construction Python's MicronParser.parse_line
// performs (MicronParser.py:358-399) so handle_link's recurse_down has widgets
// to collect. It is called from renderPage after bd.currentLines is set. The
// field widgets persist until the next renderPage (a navigation/re-render
// rebuilds them, matching Python rebuilding attr_maps on each load); during
// editing no re-render occurs, so typed text is retained across focus moves.
func (bd *BrowserDisplay) buildLineFields(lines []*micron.StyledLine) {
	bd.lineFields = make([][]*renderedField, len(lines))
	bd.radioGroups = map[string]*RadioGroup{}
	width := bd.renderedWidth
	if width <= 0 {
		width = 60
	}
	// TEMPORARY: log field-span count + specs per render to diagnose whether
	// the ICP Board search page's `query` field becomes a Field span. Remove.
	totalFields := 0
	var fieldSpecs []string
	var allLines []string
	for i, line := range lines {
		if line == nil || line.Divider {
			continue
		}
		allLines = append(allLines, fmt.Sprintf("L%d=%q", i, bd.linePlainText(i)))
		// Display-width cursor mirroring the leading spaces StyledLinesToTviewText
		// emits: indent, then alignment pad for `c`/`r` lines.
		col := line.Indent
		if line.Align == micron.AlignCenter || line.Align == micron.AlignRight {
			textWidth := 0
			for _, s := range line.Spans {
				textWidth += runewidth.StringWidth(s.Text)
			}
			avail := width - line.Indent
			avail = max(avail, textWidth)
			pad := 0
			switch line.Align {
			case micron.AlignCenter:
				pad = (avail - textWidth) / 2
			case micron.AlignRight:
				pad = avail - textWidth
			}
			col += max(pad, 0)
		}
		runeOff := 0
		for _, span := range line.Spans {
			rlen := utf8.RuneCountInString(span.Text)
			if span.Field != nil {
				rf := bd.newRenderedField(span.Field, col)
				rf.runeStart = runeOff
				rf.runeEnd = runeOff + rlen
				bd.lineFields[i] = append(bd.lineFields[i], rf)
				totalFields++
				fieldSpecs = append(fieldSpecs, fmt.Sprintf("L%d:%s/%q/%q", i, span.Field.Type, span.Field.Name, span.Field.Data))
			}
			col += runewidth.StringWidth(span.Text)
			runeOff += rlen
		}
	}
	if f, err := os.OpenFile("/tmp/peek-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		_, _ = fmt.Fprintf(f, "BUILDFIELDS total=%d specs=%v\n", totalFields, fieldSpecs)
		if totalFields > 0 {
			_, _ = fmt.Fprintf(f, "LINES %v\n", allLines)
		}
		_ = f.Close()
	}
}

// newRenderedField builds the interactive widget for one field spec. Text
// fields use a ReadlineEdit (emacs keys + hardware caret, shared kill ring) so
// the overlay behaves like Python's ReadlineEdit; checkbox/radio fields reuse
// NewFieldWidget's *tview.Checkbox (with RadioGroup mutual-exclusion wiring).
func (bd *BrowserDisplay) newRenderedField(spec *micron.FieldSpec, startCol int) *renderedField {
	width := spec.Width
	if width == 0 {
		width = defaultFieldWidth
	}
	rf := &renderedField{spec: spec, startCol: startCol, width: width}
	switch spec.Type {
	case "checkbox", "radio":
		fw := NewFieldWidget(spec, bd.radioGroups)
		if cb, ok := fw.Primitive.(*tview.Checkbox); ok {
			rf.checkbox = cb
		}
	default: // text field
		re := NewReadlineEdit(bd.app.killRing, "", "")
		re.SetText(spec.Data)
		re.SetFieldWidth(width)
		if spec.Masked {
			re.SetMaskCharacter(maskCharacter)
		}
		// Hand navigation keys back to the browser nav model so Down/Up/Tab move
		// the Pile focus off the field (Python: the Edit returns these to the
		// Pile/Scrollable). Set once at build time; the editor persists across
		// mount/unmount.
		re.onExit = func(key tcell.Key) { bd.moveFieldFocus(key) }
		rf.editor = re
	}
	return rf
}

// collectFields builds the request-data map for a form-submit link, mirroring
// Python Browser.handle_link's recurse_down (Browser.py:216-268). linkFields is
// the link's pipe-separated field-names component (the 3rd backtick segment of
// a micron submit link). Entries containing "=" become var_<k>=<v> entries;
// other entries name the fields to collect; "*" collects every field. Returns
// nil when linkFields is empty (a plain link → no request data, cache path);
// otherwise a non-nil map (matching Python's request_data = {} when link_data
// is present, so the page cache is skipped and the target is re-fetched).
func (bd *BrowserDisplay) collectFields(linkFields string) map[string]string {
	if linkFields == "" {
		return nil
	}
	rd := map[string]string{}
	all := false
	want := map[string]bool{}
	for e := range strings.SplitSeq(linkFields, "|") {
		if strings.Contains(e, "=") {
			c := strings.SplitN(e, "=", 2)
			if len(c) == 2 {
				rd["var_"+c[0]] = c[1]
			}
			continue
		}
		if e == "*" {
			all = true
			continue
		}
		if e != "" {
			want[e] = true
		}
	}
	for _, row := range bd.lineFields {
		for _, rf := range row {
			if !all && !want[rf.spec.Name] {
				continue
			}
			key := "field_" + rf.spec.Name
			switch rf.spec.Type {
			case "radio":
				// Python: only the selected radio contributes its field_value.
				if rf.checkbox.IsChecked() && rf.spec.Value != "" {
					rd[key] = rf.spec.Value
				}
			case "checkbox":
				// Python: each checked checkbox contributes field_value
				// (default "1"); same-named checkboxes concatenate with ",".
				if rf.checkbox.IsChecked() {
					val := rf.spec.Value
					if val == "" {
						val = "1"
					}
					if existing, ok := rd[key]; ok {
						rd[key] = existing + "," + val
					} else {
						rd[key] = val
					}
				}
			default: // text
				rd[key] = rf.editor.GetText()
			}
		}
	}
	return rd
}

// syncFieldFocus mounts or unmounts the text-field overlay to mirror the
// current focusLine, mirroring Python's Pile focusing the first selectable
// child of a field row's Columns (the Edit; the label text is non-selectable).
// Called after every focusLine/cursor change in handleNavKey.
//
// When the part-cursor sits on a text-field span, that field's ReadlineEdit is
// mounted and takes focus (so tview routes typing there). When the cursor sits
// on a non-selectable label part of a line that contains a text field, the
// cursor is advanced onto the field span and the field mounts — matching
// Python's Columns, which skips non-selectable text children. When the cursor
// sits on a link part, the overlay is unmounted and focus stays on bd.content
// so link navigation works. Lines with no text field always unmount.
func (bd *BrowserDisplay) syncFieldFocus() {
	if bd.focusLine < 0 || bd.focusLine >= len(bd.lineFields) {
		bd.unmountFieldOverlay()
		return
	}
	row := bd.lineFields[bd.focusLine]
	if len(row) == 0 {
		bd.unmountFieldOverlay()
		return
	}
	cur := 0
	if bd.focusLine < len(bd.lineCursors) {
		cur = bd.lineCursors[bd.focusLine]
	}
	// Cursor already on a text-field span?
	var tf *renderedField
	for _, rf := range row {
		if rf.editor != nil && rf.runeStart <= cur && cur < rf.runeEnd {
			tf = rf
			break
		}
	}
	if tf == nil {
		// If the cursor is on a link, leave link nav alone. Otherwise (label or
		// other non-field part) advance to the first text field on the line,
		// mirroring Python Columns focusing the first selectable child.
		if bd.lineLinkAtCursor(bd.focusLine) != nil {
			bd.unmountFieldOverlay()
			return
		}
		for _, rf := range row {
			if rf.editor != nil {
				tf = rf
				break
			}
		}
		if tf != nil && bd.focusLine < len(bd.lineCursors) {
			bd.lineCursors[bd.focusLine] = tf.runeStart
		}
	}
	if tf == nil {
		bd.unmountFieldOverlay()
		return
	}
	// Already mounted for this same field: nothing to do.
	if bd.fieldOverlay == tf.editor && bd.fieldOverlayLine == bd.focusLine {
		return
	}
	bd.fieldOverlay = tf.editor
	bd.fieldOverlayLine = bd.focusLine
	// NOTE: focus is intentionally LEFT on bd.content (not moved to the editor).
	// The ReadlineEdit overlay is drawn off the primitive tree by drawFieldOverlay,
	// so tview's input cascade (root → ... → bd.layout) can never reach an
	// off-tree focused primitive — SetFocus(editor) would orphan focus, the
	// cascade would stop finding the editor in-tree, and typed runes would be
	// dropped. Instead bd.handleInput (bd.layout's InputCapture, which the
	// in-tree cascade always reaches while bd.content has focus) forwards keys
	// to the overlay's handleKey. The overlay is "logically" focused
	// (bd.fieldOverlay != nil); tview focus stays on bd.content.
}

// unmountFieldOverlay clears the mounted text-field overlay without moving
// focus (focus restoration is the caller's job).
func (bd *BrowserDisplay) unmountFieldOverlay() {
	bd.fieldOverlay = nil
	bd.fieldOverlayLine = -1
}

// moveFieldFocus is the overlay's onExit handler: invoked by the ReadlineEdit
// when Down/Up/Tab is pressed while editing, it moves the Pile focus (focusLine)
// to the next/previous selectable line — mirroring Python's Pile taking the
// "down"/"up" returned by the Edit — then hands focus back to bd.content so
// syncFieldFocus either re-mounts the new line's field or restores nav mode.
func (bd *BrowserDisplay) moveFieldFocus(key tcell.Key) {
	bd.unmountFieldOverlay()
	if bd.focusLine >= 0 && bd.focusLine < len(bd.lineCursors) {
		bd.lineCursors[bd.focusLine] = 0
	}
	switch key {
	case tcell.KeyDown, tcell.KeyTab:
		if n := bd.nextSelectableLine(bd.focusLine); n >= 0 {
			bd.focusLine = n
			bd.ensureVisible()
			bd.peekLink()
		} else {
			bd.scrollDownOne()
		}
	case tcell.KeyUp:
		if p := bd.prevSelectableLine(bd.focusLine); p >= 0 {
			bd.focusLine = p
			bd.ensureVisible()
			bd.peekLink()
		} else {
			bd.scrollUpOne()
		}
	}
	if bd.app != nil {
		bd.app.SetFocus(bd.content)
	}
	bd.syncFieldFocus()
}

// drawFieldOverlay draws the mounted text-field ReadlineEdit over the field's
// screen cells. Called from browserPageView.Draw after the TextView is drawn,
// so the editor (with the typed text + caret) covers the placeholder text the
// TextView rendered underneath. The rect is recomputed each frame from the
// field's line + column and the content's scroll offset, so it tracks scrolling
// and resize. Fields are single-line (Python's urwid.Edit in a Columns does not
// wrap), so the overlay is one row tall at the field's first wrapped row.
func (bd *BrowserDisplay) drawFieldOverlay(screen tcell.Screen) {
	if bd.fieldOverlay == nil || bd.fieldOverlayLine < 0 {
		return
	}
	if bd.fieldOverlayLine >= len(bd.lineFields) {
		return
	}
	var tf *renderedField
	for _, rf := range bd.lineFields[bd.fieldOverlayLine] {
		if rf.editor == bd.fieldOverlay {
			tf = rf
			break
		}
	}
	if tf == nil {
		return
	}
	x0, y0, innerW, innerH := bd.content.GetInnerRect()
	scrollRow, _ := bd.content.GetScrollOffset()
	screenX := x0 + tf.startCol
	screenY := y0 + (bd.rowsAbove(bd.fieldOverlayLine) - scrollRow)
	if screenY < y0 || screenY >= y0+innerH || innerW <= 0 {
		return // field's row is scrolled out of view
	}
	w := tf.width
	if screenX < x0 {
		screenX = x0
	}
	if screenX+w > x0+innerW {
		w = x0 + innerW - screenX
	}
	if w < 1 {
		w = 1
	}
	bd.fieldOverlay.SetRect(screenX, screenY, w, 1)
	bd.fieldOverlay.Draw(screen)
}

// lineFieldAtCursor returns the rendered field whose span contains the line's
// part-cursor, or nil. Used by handleNavKey to toggle a checkbox/radio field on
// Space/Enter (text fields are handled by the overlay, not this path).
func (bd *BrowserDisplay) lineFieldAtCursor(idx int) *renderedField {
	if idx < 0 || idx >= len(bd.lineFields) || idx >= len(bd.lineCursors) {
		return nil
	}
	cursor := bd.lineCursors[idx]
	for _, rf := range bd.lineFields[idx] {
		if rf.runeStart <= cursor && cursor < rf.runeEnd {
			return rf
		}
	}
	return nil
}

// toggleFieldAtCursor toggles the checkbox/radio field at the focused line's
// cursor, mirroring Python's CheckBox/RadioButton keypress on Space/Enter. For
// radio, the RadioGroup SetChangedFunc wiring (NewFieldWidget) unchecks siblings.
// Returns true if a checkbox/radio was toggled (the key is consumed).
func (bd *BrowserDisplay) toggleFieldAtCursor() bool {
	rf := bd.lineFieldAtCursor(bd.focusLine)
	if rf == nil || rf.checkbox == nil {
		return false
	}
	rf.checkbox.SetChecked(!rf.checkbox.IsChecked())
	return true
}
