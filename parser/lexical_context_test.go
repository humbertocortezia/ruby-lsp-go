package parser

import "testing"

func TestLocalBindingsDistinguishValuesFromCommandArguments(t *testing.T) {
	for _, tc := range []struct {
		source  string
		regexes int
	}{
		{"value = 12; value /count", 0},
		{"def divide(value, count)\nvalue /count\nend", 0},
		{"def divide(value = 12, count = 2)\nvalue /count\nend", 0},
		{"def divide value, count\nvalue /count\nend", 0},
		{"value = 12; object.value /pattern/", 1},
		{"value = 12\ndef method\nvalue /pattern/\nend", 1},
		{"def method(value)\nend\nvalue /pattern/", 1},
		{"value = 12\nclass Scope\nvalue /pattern/\nend", 1},
		{"if ready\nvalue = 12\nend\nvalue /count", 0},
		{"value = 12 if ready\nvalue /count", 0},
		{"while ready do\nvalue = 12\nend\nvalue /count", 0},
		{"for value in list do\nvalue /count\nend", 0},
		{"list.each do |value|\nvalue /count\nend\nvalue /pattern/", 1},
		{"list.each { |value| value /count }\nvalue /pattern/", 1},
		{"object.class /pattern/", 1},
	} {
		t.Run(tc.source, func(t *testing.T) {
			tokens, err := Tokenize(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, token := range tokens {
				if token.Type == TokenRegex {
					count++
				}
			}
			if count != tc.regexes {
				t.Fatalf("wrong regex/division classification: want %d regexes, got %d", tc.regexes, count)
			}
		})
	}
}
