package rulesets

import (
	"errors"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/subscription"
)

func TestNodeOpsDecodeDefaults(t *testing.T) {
	c := Builtin()
	p, err := DecodeProfile(c, []byte(`{"template":"minimal"}`), def)
	if err != nil {
		t.Fatal(err)
	}
	n := p.Nodes
	if n.Include == nil || n.Exclude == nil || n.Types == nil || n.Regions == nil || n.Rename == nil ||
		n.Emoji != subscription.EmojiKeep || n.Sort != subscription.SortDefault || !n.IsZero() {
		t.Fatalf("defaults = %+v", n)
	}
	if err := ValidateProfile(c, p); err != nil {
		t.Fatal(err)
	}
	p, err = DecodeProfile(c, []byte(`{"nodes":{"regions":[" hk","uk"],"rename":[{"match":"(\\d+)$","replace":"#$1"}],"sort":"region","emoji":"add","dedupe":true,"limit":5}}`), def)
	if err != nil {
		t.Fatal(err)
	}
	if p.Nodes.Regions[0] != "HK" || p.Nodes.Regions[1] != "UK" || p.Nodes.Rename[0].Replace != "#$1" || p.Nodes.IsZero() {
		t.Fatalf("decoded = %+v", p.Nodes)
	}
	if err := ValidateProfile(c, p); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeProfile(c, []byte(`{"nodes":{"bogus":1}}`), def); err == nil {
		t.Fatal("unknown node-ops field accepted")
	}
	// ?profile=<template> keeps the stored node operations.
	stored := p
	r, err := ResolveProfile(c, &stored, def, Overrides{Template: "full"})
	if err != nil || r.Nodes.Limit != 5 || r.Template != "full" {
		t.Fatalf("resolve = %+v %v", r.Nodes, err)
	}
}

func TestNodeOpsValidation(t *testing.T) {
	c := Builtin()
	many := func(n int, v string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	rules := func(n int) []subscription.RenameRule {
		out := make([]subscription.RenameRule, n)
		for i := range out {
			out[i] = subscription.RenameRule{Match: "a", Replace: "b"}
		}
		return out
	}
	cases := []struct {
		ops    subscription.NodeOps
		fields []string
	}{
		{subscription.NodeOps{Include: []string{"(["}}, []string{"nodes.include[0]"}},
		{subscription.NodeOps{Exclude: []string{""}}, []string{"nodes.exclude[0]"}},
		{subscription.NodeOps{Include: []string{`[\pL\pN]{1000}`}}, []string{"nodes.include[0]"}},
		{subscription.NodeOps{Include: []string{strings.Repeat("a", 257)}}, []string{"nodes.include[0]"}},
		{subscription.NodeOps{Include: many(20, "a"), Exclude: many(13, "b")}, []string{"nodes.exclude"}},
		{subscription.NodeOps{Types: []string{"vmess", "wireguard"}}, []string{"nodes.types[1]"}},
		{subscription.NodeOps{Types: many(17, "ss")}, []string{"nodes.types"}},
		{subscription.NodeOps{Regions: []string{"HK", "XX"}}, []string{"nodes.regions[1]"}},
		{subscription.NodeOps{Rename: rules(65)}, []string{"nodes.rename"}},
		{subscription.NodeOps{Rename: []subscription.RenameRule{{Match: "(", Replace: "x"}, {Match: "a", Replace: strings.Repeat("x", 257)}, {Match: "a", Replace: " "}}},
			[]string{"nodes.rename[0].match", "nodes.rename[1].replace", "nodes.rename[2].replace"}},
		{subscription.NodeOps{Prefix: strings.Repeat("前", 33), Suffix: "\x00"}, []string{"nodes.prefix", "nodes.suffix"}},
		{subscription.NodeOps{Emoji: "flag", Sort: "random", Limit: 1001}, []string{"nodes.emoji", "nodes.sort", "nodes.limit"}},
		{subscription.NodeOps{Limit: -1}, []string{"nodes.limit"}},
	}
	for i, tc := range cases {
		p := DefaultProfile(c, def)
		p.Nodes = tc.ops.Normalized()
		if tc.ops.Emoji != "" {
			p.Nodes.Emoji = tc.ops.Emoji
		}
		err := ValidateProfile(c, p)
		var fe FieldErrors
		if !errors.As(err, &fe) {
			t.Fatalf("case %d: err = %v", i, err)
		}
		for _, f := range tc.fields {
			if len(fe[f]) == 0 {
				t.Errorf("case %d: no error on %s (got %v)", i, f, fe)
			}
		}
		if len(fe) != len(tc.fields) {
			t.Errorf("case %d: errors %v, want only %v", i, fe, tc.fields)
		}
	}
}
