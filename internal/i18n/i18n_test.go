package i18n

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	"shop/web"
)

func load(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEveryLanguageHasExactlyTheEnglishKeys(t *testing.T) {
	c := load(t)
	en := c.Keys("en")
	for _, lang := range Langs {
		got := c.Keys(lang)
		for k := range en {
			if !got[k] {
				t.Errorf("%s is missing %q", lang, k)
			}
		}
		for k := range got {
			if !en[k] {
				t.Errorf("%s has %q, which English does not", lang, k)
			}
		}
	}
}

func TestTranslationsAreFilledAndKeepTheirPlaceholders(t *testing.T) {
	c := load(t)
	for key, enText := range c.Table("en") {
		want := Placeholders(enText)
		for _, lang := range Langs {
			text := c.T(lang, key)
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s %q is empty", lang, key)
			}
			got := Placeholders(text)
			for p := range want {
				if !got[p] {
					t.Errorf("%s %q lost the placeholder {%s}: %q", lang, key, p, text)
				}
			}
			for p := range got {
				if !want[p] {
					t.Errorf("%s %q has a placeholder {%s} the code never fills: %q", lang, key, p, text)
				}
			}
		}
	}
}

// Keys the code builds at run time ("reason." + value), which the scan below
// cannot see as literals.
var dynamicKeys = []string{
	"reason.restock", "reason.recount", "reason.damaged", "reason.other",
	"movement.initial", "movement.sale", "movement.void", "movement.adjust",
}

var (
	tmplKey = regexp.MustCompile(`\{\{t "([^"]+)"\}\}`)
	jsKey   = regexp.MustCompile(`\bt(?:Or)?\("([^"]+)"`)
)

// usedKeys returns every literal key the templates and scripts ask for.
func usedKeys(t *testing.T) map[string]string {
	t.Helper()
	used := map[string]string{}
	scan := func(fsys fs.FS, dir string, re *regexp.Regexp) {
		err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			for _, m := range re.FindAllStringSubmatch(string(data), -1) {
				// "x." / "x_" are the literal head of a built-up key.
				if strings.HasSuffix(m[1], ".") || strings.HasSuffix(m[1], "_") {
					continue
				}
				used[m[1]] = path
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	scan(web.Templates, "templates", tmplKey)
	scan(web.Static, "static", jsKey)
	return used
}

func TestEveryKeyUsedByThePagesExists(t *testing.T) {
	en := load(t).Keys("en")
	used := usedKeys(t)
	if len(used) < 150 { // guards against a scan that silently matches nothing
		t.Fatalf("only %d keys found in the templates and scripts; the scan is broken", len(used))
	}
	for key, where := range used {
		if !en[key] {
			t.Errorf("%s uses translation key %q, which does not exist", where, key)
		}
	}
	for _, key := range dynamicKeys {
		if !en[key] {
			t.Errorf("dynamic key %q does not exist", key)
		}
	}
}

func TestNoUnusedKeys(t *testing.T) {
	used := usedKeys(t)
	for _, k := range dynamicKeys {
		used[k] = "dynamic"
	}
	var unused []string
	for k := range load(t).Keys("en") {
		if _, ok := used[k]; !ok {
			unused = append(unused, k)
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		t.Errorf("translation keys nothing uses (delete them): %v", unused)
	}
}

func TestFallbackAndMissingKey(t *testing.T) {
	c := load(t)
	if got := c.T("xx", "nav.sell"); got != c.T("en", "nav.sell") {
		t.Errorf("unknown language should fall back to English, got %q", got)
	}
	if got := c.T("uz", "no.such.key"); got != "no.such.key" {
		t.Errorf("missing key should show itself, got %q", got)
	}
	if c.T("uz", "nav.sell") == c.T("en", "nav.sell") {
		t.Error("Uzbek should differ from English for nav.sell")
	}
}
