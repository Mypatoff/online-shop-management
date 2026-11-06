// Package i18n holds the UI translations (English, Uzbek, Russian). Pages are
// rendered with them on the server ({{t "key"}}) and the same tables are handed
// to the browser for the strings the scripts build.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
)

//go:embed en.json uz.json ru.json
var files embed.FS

// Default is the language every other one falls back to for a missing key.
const Default = "en"

// Langs lists the supported languages, in the order the switcher shows them.
var Langs = []string{"en", "uz", "ru"}

// Names are the languages' own names, as shown in the switcher.
var Names = map[string]string{"en": "English", "uz": "O'zbekcha", "ru": "Русский"}

// LocaleTags are the BCP 47 tags the browser formats dates with.
var LocaleTags = map[string]string{"en": "en-GB", "uz": "uz-UZ", "ru": "ru-RU"}

// Catalog is every language's table.
type Catalog struct {
	tables map[string]map[string]string
}

// Load parses the embedded tables.
func Load() (*Catalog, error) {
	c := &Catalog{tables: map[string]map[string]string{}}
	for _, lang := range Langs {
		raw, err := files.ReadFile(lang + ".json")
		if err != nil {
			return nil, err
		}
		var table map[string]string
		if err := json.Unmarshal(raw, &table); err != nil {
			return nil, fmt.Errorf("i18n %s.json: %w", lang, err)
		}
		c.tables[lang] = table
	}
	return c, nil
}

// Valid reports whether lang is a supported language code.
func Valid(lang string) bool {
	_, ok := Names[lang]
	return ok
}

// T returns the text for key in lang, falling back to English and finally to
// the key itself, so a missing translation is visible but never blank.
func (c *Catalog) T(lang, key string) string {
	if s, ok := c.tables[lang][key]; ok && s != "" {
		return s
	}
	if s, ok := c.tables[Default][key]; ok {
		return s
	}
	return key
}

// Table returns lang's full table with English filling any gaps. The result
// is a copy.
func (c *Catalog) Table(lang string) map[string]string {
	out := make(map[string]string, len(c.tables[Default]))
	for k, v := range c.tables[Default] {
		out[k] = v
	}
	for k, v := range c.tables[lang] {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// Keys returns the keys of lang's own table (without the English fallback).
func (c *Catalog) Keys(lang string) map[string]bool {
	out := map[string]bool{}
	for k := range c.tables[lang] {
		out[k] = true
	}
	return out
}

var placeholder = regexp.MustCompile(`\{(\w+)\}`)

// Placeholders returns the {name} placeholders used in s.
func Placeholders(s string) map[string]bool {
	out := map[string]bool{}
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		out[m[1]] = true
	}
	return out
}
