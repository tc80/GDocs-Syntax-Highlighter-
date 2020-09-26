package parser

import (
	"GDocs-Syntax-Highlighter/request"
	"GDocs-Syntax-Highlighter/style"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"google.golang.org/api/docs/v1"
)

// Function to check if a particular
// rune is desired
type isRuneFunc func(r rune) bool

// Returns a function that will
// return true for any rune
func anyRune() isRuneFunc {
	return func(r1 rune) bool {
		return true
	}
}

// Returns a function that will
// return true only for the
// specified rune
func isRune(r1 rune) isRuneFunc {
	return func(r2 rune) bool {
		return r1 == r2
	}
}

// Gets the utf16 start and end indices of a target substring
// located in a utf8 string with a particular starting index offset.
func getUTF16SubstrIndices(target, utf8 string, offset int64) (startIndex, endIndex int64) {
	index := strings.Index(utf8, target)
	if index == -1 {
		panic(fmt.Sprintf("target `%s` not found in `%s`", target, utf8))
	}

	// add utf16 sizes until we reach the target's start
	startIndex += offset
	for _, r := range utf8[:index] {
		startIndex += GetUtf16RuneSize(r)
	}

	// endIndex is startIndex + utf16 size of target
	endIndex = startIndex
	for _, r := range target {
		endIndex += GetUtf16RuneSize(r)
	}

	return
}

// GetUtf16RuneSize gets the size of a rune in UTF-16 format
func GetUtf16RuneSize(r rune) int64 {
	rUtf16 := utf16.Encode([]rune{r}) // convert to utf16, since indices in GDocs API are utf16
	return int64(len(rUtf16))         // size of rune in utf16 format
}

// GetUtf16StringSize gets the size of a string in UTF-16 format
func GetUtf16StringSize(s string) int64 {
	var size int64
	for _, r := range s {
		size += GetUtf16RuneSize(r)
	}
	return size
}

// MapToUTF16 maps the instance's utf8 non-empty Code string to utf16 rune indices + an offset.
// Also sets the EndIndex in case it changed during any formatting.
func (c *CodeInstance) MapToUTF16() {
	if c.Code == "" {
		// Even with an empty Document it appears Google Docs
		// still has the newline character `\n`, so this
		// should never happen.
		panic("code must not be empty")
	}

	utf16Index := *c.StartIndex
	for i, r := range c.Code {
		utf16Width := GetUtf16RuneSize(r)

		// map zero-based utf8 -> utf16 + offset
		c.toUTF16[i] = utf16Index
		utf16Index += utf16Width
	}
	c.EndIndex = &utf16Index
}

// Replace gets the requests to replace all matches of a regex with a particular string.
// It also updates the instance.Code and EndIndex respectively.
func (c *CodeInstance) Replace(s *style.Shortcut) (reqs []*docs.Request) {
	for {
		if res := s.Regex.FindStringSubmatchIndex(c.Code); res != nil {
			utf8DelStart, utf8DelEnd := res[0], res[1]
			utf16DelStart, utf16DelEnd := getUTF16SubstrIndices(c.Code[utf8DelStart:utf8DelEnd], c.Code, *c.StartIndex)

			// delete target and insert replacement string
			utf16DelRange := request.GetRange(utf16DelStart, utf16DelEnd, "")
			reqs = append(reqs, request.Delete(utf16DelRange))
			reqs = append(reqs, request.Insert(s.Replace, utf16DelStart))

			// update end index for utf16 difference
			utf16InsSize := GetUtf16StringSize(s.Replace)
			newEndIndex := *c.EndIndex + utf16InsSize - (utf16DelEnd - utf16DelStart)
			c.EndIndex = &newEndIndex

			// replace c.Code
			c.Code = c.Code[:utf8DelStart] + s.Replace + c.Code[utf8DelEnd:]
			continue
		}
		return
	}
}

// Highlight gets the requests to highlight all matches of a regex with a particular color.
func (c *CodeInstance) Highlight(r *regexp.Regexp, color *docs.Color, segmentID string) (reqs []*docs.Request) {
	if results := r.FindAllStringSubmatchIndex(c.Code, -1); results != nil {
		for _, res := range results {
			utf8Start, utf8End := res[0], res[1]
			utf16Size := GetUtf16StringSize(c.Code[utf8Start:utf8End])
			utf16StartOffset := c.toUTF16[utf8Start]
			utf16Range := request.GetRange(utf16StartOffset, utf16StartOffset+utf16Size, segmentID)
			reqs = append(reqs, request.UpdateForegroundColor(color, utf16Range))
		}
	}
	return
}
