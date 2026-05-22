package present

import (
	"encoding/json"
	"fmt"
	"strings"
)

// QuestionImage is stored in questions.image_urls JSONB (array of objects or legacy strings).
type QuestionImage struct {
	URL         string `json:"url"`
	Position    string `json:"position"`
	OptionIndex *int   `json:"option_index,omitempty"`
}

// ParseQuestionImages reads image_urls JSONB (string[] or []QuestionImage).
func ParseQuestionImages(raw *string) []QuestionImage {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	data := []byte(*raw)

	var asStrings []string
	if err := json.Unmarshal(data, &asStrings); err == nil {
		out := make([]QuestionImage, 0, len(asStrings))
		for _, u := range asStrings {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			out = append(out, QuestionImage{URL: u, Position: "below"})
		}
		return out
	}

	var asObjs []QuestionImage
	if err := json.Unmarshal(data, &asObjs); err == nil {
		out := make([]QuestionImage, 0, len(asObjs))
		for _, img := range asObjs {
			img.URL = strings.TrimSpace(img.URL)
			if img.URL == "" {
				continue
			}
			pos := strings.ToLower(strings.TrimSpace(img.Position))
			switch pos {
			case "above", "below", "after", "option":
				img.Position = pos
			default:
				img.Position = "below"
			}
			out = append(out, img)
		}
		return out
	}
	return nil
}

// FlatImageURLs returns unique URLs in order.
func FlatImageURLs(imgs []QuestionImage) []string {
	if len(imgs) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(imgs))
	for _, img := range imgs {
		if img.URL == "" {
			continue
		}
		if _, ok := seen[img.URL]; ok {
			continue
		}
		seen[img.URL] = struct{}{}
		out = append(out, img.URL)
	}
	return out
}

// MarshalQuestionImages stores normalized JSON for DB.
func MarshalQuestionImages(imgs []QuestionImage) (*string, error) {
	if len(imgs) == 0 {
		return nil, nil
	}
	norm := make([]QuestionImage, 0, len(imgs))
	for _, img := range imgs {
		img.URL = strings.TrimSpace(img.URL)
		if img.URL == "" {
			continue
		}
		pos := strings.ToLower(strings.TrimSpace(img.Position))
		if pos != "above" && pos != "below" && pos != "after" && pos != "option" {
			pos = "below"
		}
		img.Position = pos
		norm = append(norm, img)
	}
	if len(norm) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(norm)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}

// NormalizeImageURLsInput accepts JSON array of strings or objects from API body.
func NormalizeImageURLsInput(raw json.RawMessage) (*string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var asStrings []string
	if err := json.Unmarshal(raw, &asStrings); err == nil {
		imgs := make([]QuestionImage, 0, len(asStrings))
		for _, u := range asStrings {
			u = strings.TrimSpace(u)
			if u != "" {
				imgs = append(imgs, QuestionImage{URL: u, Position: "below"})
			}
		}
		return MarshalQuestionImages(imgs)
	}
	var asObjs []QuestionImage
	if err := json.Unmarshal(raw, &asObjs); err == nil {
		return MarshalQuestionImages(asObjs)
	}
	return nil, fmt.Errorf("image_urls must be a JSON array of URL strings or objects {url, position}")
}
