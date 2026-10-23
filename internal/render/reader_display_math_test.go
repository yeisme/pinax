package render

import (
	"reflect"
	"strings"
	"testing"
)

func TestReaderDocumentRichDisplayMath(t *testing.T) {
	for _, test := range []struct{ body, source string }{
		{"$$ x+y $$\n", " x+y "},
		{"$$\n\\frac{a_1}{b_2} + \\text{中文}\n$$\n", "\n\\frac{a_1}{b_2} + \\text{中文}\n"},
		{"$$\r\na+b  \r\n$$\r\n", "\r\na+b  \r\n"},
		{"$$\\text{price \\$5}$$", "\\text{price \\$5}"},
	} {
		t.Run(test.body, func(t *testing.T) {
			rich, err := BuildReaderRichDocument(test.body)
			if err != nil {
				t.Fatal(err)
			}
			if len(rich.Annotations) != 1 || rich.Annotations[0].Kind != "math" || rich.Annotations[0].Source != test.source {
				t.Fatalf("display math source mismatch: %#v", rich.Annotations)
			}
			legacy, err := BuildReaderDocument(test.body)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(legacy, rich.Document) {
				t.Fatal("display math changed legacy document")
			}
			if LocateReaderAnchor(rich.Document, rich.Annotations[0].Anchor) != 0 {
				t.Fatal("display math anchor mismatch")
			}
		})
	}
}

func TestReaderDocumentRichDisplayMathRefusesAmbiguousInput(t *testing.T) {
	for _, body := range []string{
		"$5 and $10", "before $$x$$", "$$x$$ after", "$$unclosed", "\\$$escaped$$", "$$$x$$$", "$$$$",
		"$$x$$ $$y$$", "$$\nx\n\ny\n$$", "`$$x$$`", "```text\n$$x$$\n```", "> $$x$$", "- $$x$$",
		"$$[link](https://example.invalid)$$", "$$`code`$$", "$$<script>x</script>$$", "$$x\\$$",
	} {
		t.Run(body, func(t *testing.T) {
			rich, err := BuildReaderRichDocument(body)
			if err != nil {
				t.Fatal(err)
			}
			if len(rich.Annotations) != 0 {
				t.Fatalf("ambiguous display math accepted: %#v", rich.Annotations)
			}
		})
	}
	if _, err := BuildReaderRichDocument("$$" + strings.Repeat("字", 23000) + "$$"); err == nil {
		t.Fatal("display math byte budget bypassed")
	}
}
