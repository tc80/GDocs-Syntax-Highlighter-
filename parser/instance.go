package parser

import (
	"GDocs-Syntax-Highlighter/request"
	"GDocs-Syntax-Highlighter/style"
	"log"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"
)

const (
	codeInstanceStart = "<code>"  // required tag to denote start of code instance
	codeInstanceEnd   = "</code>" // required tag to denote end of code instance
	configStart       = "<conf>"  // required tag to denote start of config
	configEnd         = "</conf>" // required tag to denote end of config
)

var (
	// Optional directive to specify the language of the code.
	// If not set, #lang=go is assumed by default.
	configLangRegex = regexp.MustCompile("^#lang=([\\w_]+)$")
)

// CodeInstance describes a section in the Google Doc
// that has a config and code fragment.
type CodeInstance struct {
	builder          strings.Builder // string builder for code body
	foundConfigStart bool            // whether the config start tag was found
	foundConfigEnd   bool            // whether the config end tag was found
	toUTF16          map[int]int64   // maps the indices of the zero-based utf8 rune in Code to utf16 rune indices+start utf16 offset
	Code             string          // the code as text
	Theme            *string         // theme
	Font             *string         // font
	FontSize         *float64        // font size
	Lang             *style.Language // the coding language
	StartIndex       int64           // utf16 start index of code
	EndIndex         int64           // utf16 end index of code
	Shortcuts        *bool           // whether shortcuts are enabled
	Format           *style.Format   // whether we are being requested to format the code
}

// GetRange gets the *docs.Range
// for a particular code instance.
func (c *CodeInstance) GetRange() *docs.Range {
	return request.GetRange(c.StartIndex, c.EndIndex)
}

// GetTheme gets the *style.Theme for a particular code instance.
// Note that the language and theme fields must be valid.
func (c *CodeInstance) GetTheme() *style.Theme {
	return c.Lang.Themes[*c.Theme]
}

// Sets default values if unset.
func (c *CodeInstance) setDefaults() {
	if c.Lang == nil {
		c.Lang = style.GetDefaultLanguage()
	}
	if c.Format == nil {
		c.Format = &style.Format{}
	}
	if c.Font == nil {
		defaultFont := style.DefaultFont
		c.Font = &defaultFont
	}
	if c.FontSize == nil {
		defaultSize := style.DefaultFontSize
		c.FontSize = &defaultSize
	}
	if c.Theme == nil {
		defaultTheme := style.DefaultTheme
		c.Theme = &defaultTheme
	}
	if c.Shortcuts == nil {
		defaultShortcuts := style.DefaultShortcutSetting
		c.Shortcuts = &defaultShortcuts
	}
	if c.toUTF16 == nil {
		c.toUTF16 = make(map[int]int64)
	}
}

// Checks for header tags/directives in a particular
// string that is located in a *docs.ParagraphElement.
func (c *CodeInstance) checkForHeader(s string, par *docs.ParagraphElement) {
	// search for start of config tags
	if !c.foundConfigStart {
		if strings.EqualFold(s, configStart) {
			c.foundConfigStart = true
		}
		return
	}

	// check for end of config
	if strings.EqualFold(s, configEnd) {
		c.foundConfigEnd = true
		c.StartIndex = par.EndIndex
		c.setDefaults()
		return
	}

	// check for format (must be bolded)
	if c.Format == nil && strings.EqualFold(s, style.FormatDirective) {
		formatStart, formatEnd := getUTF16SubstrIndices(style.FormatDirective, par.TextRun.Content, par.StartIndex)
		c.Format = &style.Format{
			Bold:       par.TextRun.TextStyle.Bold,
			StartIndex: formatStart,
			EndIndex:   formatEnd,
		}
		return
	}

	// check for shortcuts (must be bolded)
	if c.Shortcuts == nil && strings.EqualFold(s, style.ShortcutsDirective) {
		c.Shortcuts = &par.TextRun.TextStyle.Bold
		return
	}

	// check for language
	if c.Lang == nil {
		if res := configLangRegex.FindStringSubmatch(s); len(res) == 2 {
			if lang, ok := style.GetLanguage(res[1]); ok {
				c.Lang = lang
			} else {
				// TODO: maybe add a comment to the Google Doc
				// in the future to notify of an invalid language name
				log.Printf("Unknown language: `%s`\n", res[1])
			}
			return
		}
	}

	// check for font
	if c.Font == nil {
		if res := style.FontRegex.FindStringSubmatch(s); len(res) == 2 {
			if font, ok := style.GetFont(res[1]); ok {
				c.Font = &font
			} else {
				// TODO: maybe add a comment to the Google Doc
				// in the future to notify of an invalid language name
				log.Printf("Unknown font: `%s`\n", res[1])
			}
			return
		}
	}

	// check for font size
	if c.FontSize == nil {
		if res := style.FontSizeRegex.FindStringSubmatch(s); len(res) == 3 {
			float, err := strconv.ParseFloat(res[1], 64)
			if err != nil {
				log.Printf("Failed to parse font size `%s` into float64: %s\n", res[1], err)
			} else {
				c.FontSize = &float // if it is 0, will default to 1
			}
			return
		}
	}

	// check for theme
	if c.Theme == nil {
		if res := style.ThemeRegex.FindStringSubmatch(s); len(res) == 2 {
			if theme, ok := style.GetTheme(res[1]); ok {
				c.Theme = &theme
			} else {
				// TODO: maybe add a comment to the Google Doc
				// in the future to notify of an invalid language name
				log.Printf("Unknown theme: `%s`\n", res[1])
			}
			return
		}
	}

	log.Printf("Unexpected config token: `%s`\n", s)
}

// GetCodeInstances gets the instances of code that will be processed in
// a Google Doc. Each instance will be surrounded with <code> and </code> tags, as
// well as a header containing info for configuration with <config> and </config> tags.
func GetCodeInstances(doc *docs.Document) []*CodeInstance {
	var instances []*CodeInstance
	var cur *CodeInstance

	for _, elem := range doc.Body.Content {
		if elem.Paragraph != nil {
			for _, par := range elem.Paragraph.Elements {
				if par.TextRun != nil {
					content := par.TextRun.Content
					italics := par.TextRun.TextStyle.Italic

					if cur == nil || !cur.foundConfigEnd {
						// iterate over each word
						for _, s := range strings.Fields(content) {
							// note: all tags must be in italics to separate them
							// from any collision with the code body
							if !italics {
								continue // ignore non-italics
							}

							// have not found start of instance yet so check for start symbol
							if cur == nil {
								if strings.EqualFold(s, codeInstanceStart) {
									cur = &CodeInstance{}
								}
								continue
							}

							cur.checkForHeader(s, par)
						}
						continue
					}

					// check for footer/end symbol
					if italics && strings.EqualFold(strings.TrimSpace(content), codeInstanceEnd) {
						cur.Code = cur.builder.String()
						instances = append(instances, cur)
						cur = nil
						continue
					}

					// write untrimmed body content, update end index
					cur.builder.WriteString(content)
					cur.EndIndex = par.EndIndex
				}
			}
		}
	}
	return instances
}