package main

import "testing"

func TestParseCallbacksStrictYAMLMigration(t *testing.T) {
	for _, input := range []string{
		"- emoji: smile\n  emoji: other\n  message: hi\n",
		"- emoji: smile\n  message: hi\n---\n- emoji: other\n  message: ignored\n",
		"- emoji: smile\n  message: hi\n---\n",
		"- emoji: smile\n  message: hi\n---\n[broken\n",
	} {
		if _, err := parseCallbacks([]byte(input)); err == nil {
			t.Errorf("accepted ambiguous YAML: %q", input)
		}
	}
	callbacks, err := parseCallbacks([]byte("- emoji: ' smile '\n  message: |\n    hello\n    world\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(callbacks) != 1 || callbacks[0].Emoji != "smile" || callbacks[0].Message != "hello\nworld\n" {
		t.Fatalf("unexpected callbacks: %#v", callbacks)
	}
}
