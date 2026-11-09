package app

import "github.com/egoist/mygo/ui"

func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconChanges  = icon(`<path d="M8 2.5v11M2.5 8h11"/><rect x="1.5" y="1.5" width="13" height="13" rx="3"/>`)
	iconHistory  = icon(`<circle cx="8" cy="8" r="2.2"/><path d="M8 1.5v4.3M8 10.2v4.3"/>`)
	iconBranch   = icon(`<circle cx="4.5" cy="3.5" r="1.6"/><circle cx="4.5" cy="12.5" r="1.6"/><circle cx="11.5" cy="5" r="1.6"/><path d="M4.5 5.1v5.8M11.5 6.6c0 3-7 2.2-7 4.3"/>`)
	iconStash    = icon(`<path d="M2 5.5h12v7a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 2 12.5z"/><path d="M1.5 2.5h13v3h-13zM6.5 8.5h3"/>`)
	iconRemote   = icon(`<circle cx="8" cy="8" r="6.5"/><path d="M1.5 8h13M8 1.5c2 2 2.6 4 2.6 6.5S10 12.5 8 14.5C6 12.5 5.4 10.5 5.4 8S6 3.5 8 1.5z"/>`)
	iconFetch    = icon(`<path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9"/><path d="M13.5 2.5v3h-3"/>`)
	iconPull     = icon(`<path d="M8 2v10M4 8.5l4 4 4-4M3 14.5h10"/>`)
	iconPush     = icon(`<path d="M8 14V4M4 7.5l4-4 4 4M3 1.5h10"/>`)
	iconCommand  = icon(`<path d="M5.5 4a1.5 1.5 0 1 0-1.5 1.5h8A1.5 1.5 0 1 0 10.5 4v8a1.5 1.5 0 1 0 1.5-1.5H4A1.5 1.5 0 1 0 5.5 12z"/>`)
	iconTerminal = icon(`<rect x="1.5" y="2.5" width="13" height="11" rx="2"/><path d="M4.5 6l2 2-2 2M8.5 10.5h3"/>`)
	iconCheck    = icon(`<path d="M3 8.5l3 3 7-7"/>`)
	iconFolder   = icon(`<path d="M1.5 4a1.5 1.5 0 0 1 1.5-1.5h3l1.5 2H13a1.5 1.5 0 0 1 1.5 1.5v6A1.5 1.5 0 0 1 13 13.5H3A1.5 1.5 0 0 1 1.5 12z"/>`)
	iconHead     = icon(`<circle cx="8" cy="8" r="3" fill="currentColor"/>`)
	iconCopy     = icon(`<rect x="5.5" y="5.5" width="9" height="9" rx="2"/><path d="M10.5 5.5V3.5a2 2 0 0 0-2-2h-5a2 2 0 0 0-2 2v5a2 2 0 0 0 2 2h2"/>`)
	iconUndo     = icon(`<path d="M5 3L2 6l3 3"/><path d="M2 6h7.5a4.5 4.5 0 0 1 0 9H6"/>`)
	iconRedo     = icon(`<path d="M11 3l3 3-3 3"/><path d="M14 6H6.5a4.5 4.5 0 0 0 0 9H10"/>`)
	iconWarning  = icon(`<path d="M8 2l6.5 11.5h-13z"/><path d="M8 6.5v3M8 11.5v.01"/>`)
)
