package scriptc

import (
	"strings"
	"testing"
)

// TestLowerJSX checks the call each JSX form lowers to.
func TestLowerJSX(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no JSX", "const a = b < c && d > e / 2\n", "const a = b < c && d > e / 2\n"},
		{"intrinsic", `const a = <panel style={s} />`, `const a = __modlockJsx("panel", { style: s })`},
		{"string attribute and flag", `x(<button data="a &amp; b" wide />)`, `x(__modlockJsx("button", { data: "a & b", wide: true }))`},
		{"text", "f(<label>\n  Hello,\n  world  </label>)", "f(__modlockJsx(\"label\", { children: \"Hello, world  \" })\n\n)"},
		{"children", `<panel>{a}<label>b</label>{/* none */}</panel>`, `__modlockJsx("panel", { children: [a, __modlockJsx("label", { children: "b" })] })`},
		{"nested expression", `<panel>{on && <image src={` + "`${p}/x`" + `} />}</panel>`, `__modlockJsx("panel", { children: on && __modlockJsx("image", { src: ` + "`${p}/x`" + ` }) })`},
		{"component", `<Card width={2} key={k}>{c}</Card>`, `__modlockJsx("fragment", { children: Card({ width: 2, children: c }) }, k)`},
		{"fragment", `<><label>a</label>b</>`, `__modlockJsx("fragment", { children: [__modlockJsx("label", { children: "a" }), "b"] })`},
		{"spread", `<panel {...rest} />`, `__modlockJsx("panel", { ...rest })`},
		{"generic arrow", `const id = <T,>(v: T) => v`, `const id = <T,>(v: T) => v`},
		{"regexp and comment", "const r = /<a>/g // <b>\n", "const r = /<a>/g // <b>\n"},
		{"type arguments", `const m = new Map<string, number>()`, `const m = new Map<string, number>()`},
		{"return", "return (\n  <label>a</label>\n)", "return (\n  __modlockJsx(\"label\", { children: \"a\" })\n)"},
		{"lines", "<panel>\n<label>a</label>\n</panel>\nnext", "__modlockJsx(\"panel\", { children: __modlockJsx(\"label\", { children: \"a\" }) })\n\n\nnext"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Lower the case.
			got, err := LowerJSX([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}

			// Compare the whole file, with its import, and its line count.
			want := c.want
			if want != c.in {
				want = jsxImport + want
			}
			if string(got) != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
			if lines := strings.Count(string(got), "\n"); lines != strings.Count(c.in, "\n") {
				t.Errorf("lowered to %d lines, want %d", lines, strings.Count(c.in, "\n"))
			}
		})
	}
}

// TestLowerJSXErrors checks that malformed JSX fails with its line.
func TestLowerJSXErrors(t *testing.T) {
	for _, in := range []string{"f(\n<panel>", "<panel></label>", "<panel a=>"} {
		if _, err := LowerJSX([]byte(in)); err == nil {
			t.Errorf("%q lowered without an error", in)
		}
	}
}
