package markdown

import (
	"strings"
	"testing"
)

func TestStripBase64ImagesRemovesInlineDataURI(t *testing.T) {
	input := "before\n![diagram](data:image/png;base64,AAAA)\nafter"
	got := StripBase64Images(input)

	if got != "before\n\nafter" {
		t.Fatalf("unexpected cleaned content: %q", got)
	}
}

func TestStripBase64ImagesRemovesReferenceStyleDataURIAndUsages(t *testing.T) {
	input := strings.Join([]string{
		"# VDS 支持组播",
		"",
		"* UI 调整![][image1]![][image2]",
		"* 保留外链![][remote]",
		"",
		"[image1]: <data:image/png;base64,AAAA>",
		"[image2]: data:image/jpeg;base64,BBBB",
		"[remote]: https://example.com/screenshot.png",
	}, "\n")

	got := StripBase64Images(input)
	if strings.Contains(got, "data:image") {
		t.Fatalf("base64 data URI was not removed: %q", got)
	}
	if strings.Contains(got, "![][image1]") || strings.Contains(got, "![][image2]") {
		t.Fatalf("base64 image references were not removed: %q", got)
	}
	if !strings.Contains(got, "![][remote]") || !strings.Contains(got, "[remote]: https://example.com/screenshot.png") {
		t.Fatalf("non-base64 image reference should be preserved: %q", got)
	}
	if !strings.Contains(got, "* UI 调整") {
		t.Fatalf("surrounding prose should be preserved: %q", got)
	}
}

func TestStripBase64ImagesNormalizesReferenceLabels(t *testing.T) {
	input := "看图![alt][ Image 1 ]\n\n[image  1]: <data:image/png;base64,AAAA>"

	got := StripBase64Images(input)
	if strings.Contains(got, "data:image") || strings.Contains(got, "![alt]") {
		t.Fatalf("reference label normalization failed: %q", got)
	}
}

func TestStripBase64ImagesHandlesCRLFReferenceDefinitions(t *testing.T) {
	input := "before![][image1]\r\n[image1]: <data:image/png;base64,AAAA>\r\nafter"

	got := StripBase64Images(input)
	if strings.Contains(got, "data:image") || strings.Contains(got, "![][image1]") {
		t.Fatalf("CRLF reference definition was not removed: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("surrounding content should be preserved: %q", got)
	}
}
