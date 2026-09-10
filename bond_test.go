package main

import "testing"

func TestParseBondOptionValue(t *testing.T) {
	cases := map[string]string{
		"layer3+4 1\n": "layer3+4",
		"fast 1\n":     "fast",
		"slow 0\n":     "slow",
		"layer2 0":     "layer2",
		"":             "unknown",
		"   \n":        "unknown",
	}
	for input, want := range cases {
		if got := parseBondOptionValue(input); got != want {
			t.Errorf("parseBondOptionValue(%q) = %q, want %q", input, got, want)
		}
	}
}

// 接口不存在时必须返回 unknown 而不是空字符串，避免回读校验误判为通过
func TestReadBondOptionMissingInterface(t *testing.T) {
	if got := readBondOption("nosuchbond999", "lacp_rate"); got != "unknown" {
		t.Errorf("readBondOption = %q, want %q", got, "unknown")
	}
}
