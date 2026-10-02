package parser

import (
	"errors"
	"strings"
	"testing"
)

func TestERBOnlyRubyAndOriginalPositions(t *testing.T) {
	source := "<%# It's a read/write helper. } \" %>\n#!/bin/sh\necho \"can't start <%= @home %>/bin\"\n<%% class NotRuby; end %>\n<p>あ</p><%- class InTemplate -%>\n<% def works; end %>\n<% end %>\n"
	scanner := NewERBScanner(source)
	ruby := scanner.RubyContent()
	if len(ruby) != len(source) || strings.Count(ruby, "\n") != strings.Count(source, "\n") {
		t.Fatalf("lost source coordinates: %q", ruby)
	}
	if strings.Contains(ruby, "NotRuby") || strings.Contains(ruby, "helper") || strings.Contains(ruby, "can't") {
		t.Fatalf("host/comment text leaked into Ruby: %q", ruby)
	}
	result, err := ParseSource(ruby)
	if err != nil || result == nil {
		t.Fatalf("wrong template AST: %v %v; ruby=%q", result, err, ruby)
	}
	foundClass := false
	for _, node := range result.AST.Children {
		if node.Name == "InTemplate" {
			foundClass = true
		}
	}
	if !foundClass {
		t.Fatalf("missing template class: %+v", result.AST.Children)
	}
	for _, name := range []string{"InTemplate", "works", "@home"} {
		offset := strings.Index(source, name)
		line, col := sourcePosition(source, offset)
		mappedLine, mappedCol := scanner.MapRubyToERB(line, col)
		if mappedLine != line || mappedCol != col || ruby[offset:offset+len(name)] != name {
			t.Fatalf("wrong mapping of %s: %d:%d -> %d:%d", name, line, col, mappedLine, mappedCol)
		}
		if scanner.InsideHostLanguage(offset) {
			t.Fatalf("Ruby delegated as host: %s", name)
		}
	}
	if !scanner.InsideHostLanguage(strings.Index(source, "can't")) || !scanner.InsideHostLanguage(strings.Index(source, "NotRuby")) || !scanner.InsideHostLanguage(strings.Index(source, "helper")) {
		t.Fatal("host/comments not delegated")
	}
}

func TestUnclosedERBTagIsAnError(t *testing.T) {
	scanner := NewERBScanner("host\n  <% class Missing")
	var incomplete *IncompleteLiteralError
	if !errors.As(scanner.Err(), &incomplete) || incomplete.Kind != "ERB tag" || incomplete.Line != 1 || incomplete.Column != 2 {
		t.Fatalf("missing opener diagnostic: %v", scanner.Err())
	}
}

func TestERBSameLineTagsRemainSeparateStatements(t *testing.T) {
	source := "<% class One; end %>text<% class Two; end %>"
	result, err := ParseSource(NewERBScanner(source).RubyContent())
	if err != nil || result == nil || len(result.AST.Children) != 2 {
		t.Fatalf("tags merged: %v %v", result, err)
	}
}
