package panorama

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/paralin/modlock/source2/kv3"
	"github.com/paralin/modlock/source2/resource"
)

// source is a layout using every kind of node the compiler writes.
const source = `<?xml version="1.0" encoding="utf-8"?>
<!-- A menu with its styles, scripts and snippets. -->
<root>
	<styles>
		<include src="s2r://panorama/styles/citadel_base_styles.vcss_c" />
	</styles>
	<scripts>
		<include src="file://{resources}/scripts/menu.js" />
		<script><![CDATA[$.Msg("a < b && c");]]></script>
	</scripts>
	<snippets>
		<snippet name="Row">
			<Label class="Row" text="#row_title" />
		</snippet>
	</snippets>
	<Panel id="Menu" style="width: 100%;" hittest="false">
		<Label text="Tom &amp; Jerry&nbsp;&quot;2&quot;" />
		<Image src="file://{resources}/images/icon.png" />
		<Panel />
	</Panel>
</root>
`

// TestLayout compiles a layout, decompiles it, and compiles the result to the
// same tree; and compiles a script to a resource holding its text.
func TestLayout(t *testing.T) {
	// Compile the layout and read its tree.
	compiled, err := Compile("mymod", "panorama/layout/menu.xml", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	first := layoutTree(t, compiled)
	root := first["m_AST"].(map[string]any)["m_pRoot"].(map[string]any)
	if root["eType"] != "ROOT" || len(root["vecChildren"].([]any)) != 4 {
		t.Fatalf("compiled root %v", root)
	}

	// Decompile it and compile the result again.
	text, err := Decompile(compiled)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Compile("mymod", "panorama/layout/menu.xml", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if second := layoutTree(t, again); !reflect.DeepEqual(first, second) {
		t.Fatalf("the decompiled layout compiles to another tree:\n%s", text)
	}

	// A script stores its text.
	script := []byte(`$.Msg("hello");`)
	compiled, err = Compile("mymod", "panorama/scripts/menu.js", script)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := resource.ReadBlock(compiled, "DATA"); err != nil || !bytes.Equal(data, script) {
		t.Fatalf("the compiled script holds %q, %v", data, err)
	}

	// A layout that does not close names its error.
	if _, err := Compile("mymod", "panorama/layout/bad.xml", []byte("<root><Panel></root>")); err == nil {
		t.Fatal("compiled a layout that does not close")
	}
}

// layoutTree decodes a compiled layout's tree without source positions.
func layoutTree(t *testing.T, compiled []byte) map[string]any {
	t.Helper()
	data, err := resource.ReadBlock(compiled, "LaCo")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := kv3.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	var strip func(any)
	strip = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			delete(value, "sourceLineColumn")
			for _, child := range value {
				strip(child)
			}
		case []any:
			for _, child := range value {
				strip(child)
			}
		}
	}
	strip(tree)
	return tree.(map[string]any)
}
