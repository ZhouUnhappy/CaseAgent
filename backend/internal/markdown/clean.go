package markdown

import (
	"regexp"
	"strings"
)

var (
	inlineBase64ImagePattern = regexp.MustCompile(`(?i)!\[[^\]\r\n]*\]\(\s*<?data:image/[^;\s>]+;base64,[^)\r\n>]*>?\s*(?:"[^"\r\n]*")?\)`)
	referenceBase64Pattern   = regexp.MustCompile(`(?im)^[ \t]{0,3}\[([^\]\r\n]+)\]:[ \t]*<?data:image/[^;\s>]+;base64,[^\r\n>]*>?[ \t]*(?:"[^"\r\n]*")?[ \t]*\r?$`)
	referenceImagePattern    = regexp.MustCompile(`!\[[^\]\r\n]*\]\[([^\]\r\n]+)\]`)
)

// StripBase64Images removes embedded image data from Markdown before storage,
// chunking, and embedding. It intentionally preserves surrounding prose because
// product docs are expected to describe the image content in text.
func StripBase64Images(content string) string {
	if content == "" {
		return ""
	}

	base64ReferenceLabels := collectBase64ReferenceLabels(content)
	cleaned := inlineBase64ImagePattern.ReplaceAllString(content, "")
	if len(base64ReferenceLabels) > 0 {
		cleaned = referenceImagePattern.ReplaceAllStringFunc(cleaned, func(match string) string {
			parts := referenceImagePattern.FindStringSubmatch(match)
			if len(parts) != 2 {
				return match
			}
			if _, ok := base64ReferenceLabels[normalizeReferenceLabel(parts[1])]; ok {
				return ""
			}
			return match
		})
	}
	cleaned = referenceBase64Pattern.ReplaceAllString(cleaned, "")

	return cleaned
}

func collectBase64ReferenceLabels(content string) map[string]struct{} {
	matches := referenceBase64Pattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	labels := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		labels[normalizeReferenceLabel(match[1])] = struct{}{}
	}
	return labels
}

func normalizeReferenceLabel(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}
