package render

import (
	"reflect"
	"strings"
	"testing"
)

func TestReaderDocumentRichEmbedCandidates(t *testing.T) {
	for _, raw := range []string{"![[Target]]", "![[notes/目标.md#标题|显示名]]", "![[#^block-id]]", "![[attachments/image.png]]"} {
		t.Run(raw, func(t *testing.T) {
			body := "# Source\n\n" + raw + "\r\n"
			rich, err := BuildReaderRichDocument(body)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := BuildReaderDocument(body)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rich.Document, plain) {
				t.Fatal("candidate parsing changed legacy document")
			}
			if len(rich.Embeds) != 1 || rich.Embeds[0].Raw != raw || rich.Embeds[0].Anchor != rich.Document.Blocks[1].Anchor {
				t.Fatalf("candidate missing or misplaced: %+v", rich.Embeds)
			}
			if len(rich.Annotations) != 0 {
				t.Fatal("unresolved candidate leaked into rich wire annotations")
			}
		})
	}
}
func TestReaderDocumentRichEmbedLiterals(t *testing.T) {
	for _, body := range []string{"`![[Target]]`", "```text\n![[Target]]\n```", "\\![[Target]]", "![[Tar\\get]]", "before ![[Target]]", "![[Target]] after", "![[A]] ![[B]]", "![[a\nb]]", "![[*Target*]]", "![[[Target](https://example.test)]]", "![[<b>Target</b>]]", "> ![[Target]]", "- ![[Target]]", "# ![[Target]]", "![[ ]]", "![[a\x00b]]"} {
		t.Run(body, func(t *testing.T) {
			rich, err := BuildReaderRichDocument(body)
			if err != nil {
				t.Fatal(err)
			}
			if len(rich.Embeds) != 0 {
				t.Fatalf("literal became live candidate: %+v", rich.Embeds)
			}
		})
	}
}
func TestReaderDocumentRichEmbedBudget(t *testing.T) {
	if _, err := BuildReaderRichDocument(strings.Repeat("![[Target]]\n\n", 32)); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{strings.Repeat("![[Target]]\n\n", 33), "![[" + strings.Repeat("x", 2048) + "]]"} {
		if _, err := BuildReaderRichDocument(body); err == nil {
			t.Fatal("embed candidate budget ignored")
		}
		if _, err := BuildReaderDocument(body); err != nil {
			t.Fatal("new budget changed legacy reader", err)
		}
	}
}
