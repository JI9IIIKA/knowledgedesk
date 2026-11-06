package knowledgedesk

import (
	"strings"
	"testing"
)

func TestSplitUnicode(t *testing.T) {
	input := "Привет мир это проверка документов"
	chunks := Split(input, 2)
	if len(chunks) != 3 || strings.Join(chunks, " ") != input {
		t.Fatalf("lost words %v", chunks)
	}
	if len(Split("", 2)) != 0 {
		t.Fatal("empty chunk")
	}
}
