// Package panorama holds the renderer that draws mods' interfaces in the
// player's game. A host whose own interface script receives the screens may
// compile Drawing from src/draw.ts into that script; vendoring this package
// delivers the source beside the ui protocol it reads. A page may draw the
// same screens in a browser on browser/dom.ts, the stand-in for game panels,
// with the layout rules in browser/panels.css.
package panorama

import "embed"

// Sources holds the drawing's TypeScript source and its browser stand-in.
//
//go:embed src/draw.ts browser/dom.ts browser/panels.css
var Sources embed.FS
