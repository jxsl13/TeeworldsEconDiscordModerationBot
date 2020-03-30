package main

import (
	"testing"
)

func Test_newNotifyMap(t *testing.T) {

	m := newNotifyMap()

	m.Add("@juan", "nameless tee")

	mentions := m.Tracked("nameless tee")

	if mentions == nil {
		t.Fatal("mentions should be non nil.")
	}

	if len(mentions) != 1 {
		t.Fatal("mentions should be of length 1")
	}

	if mentions[0] != "@juan" {
		t.Fatalf("Expected '@juan', got '%s'", mentions[0])
	}

	m.Add("@juanita", "nameless tee")
	mentions = m.Tracked("nameless tee")

	if mentions == nil {
		t.Fatal("mentions should be non nil.")
	}

	if len(mentions) != 2 {
		t.Fatal("mentions should be of length 2")
	}

	if mentions[0] != "@juan" {
		t.Fatalf("Expected '@juan', got '%s'", mentions[0])
	}

	if mentions[1] != "@juanita" {
		t.Fatalf("Expected '@juanita', got '%s'", mentions[0])
	}

	m.Remove("@juan")
	mentions = m.Tracked("nameless tee")

	if mentions == nil {
		t.Fatal("mentions should be non nil.")
	}

	if len(mentions) != 1 {
		t.Fatal("mentions should be of length 1")
	}

	if mentions[0] != "@juanita" {
		t.Fatalf("Expected '@juanita', got '%s'", mentions[0])
	}

	m.Remove("@juanita")
	mentions = m.Tracked("nameless tee")

	if len(mentions) != 0 {
		t.Fatal("mentions should be EMPTY")
	}

}
