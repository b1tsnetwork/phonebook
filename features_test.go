package main

import "testing"

func TestUnitDepthsAndAbsences(t *testing.T) {
	d := unitDepths([]OrgUnit{{ID: 1}, {ID: 2, Parent: 1}, {ID: 3, Parent: 2}, {ID: 4, Parent: 99}})
	if d[1] != 1 || d[2] != 2 || d[3] != 3 || d[4] != 1 {
		t.Fatalf("Tiefen: %v", d)
	}
	cur := currentAbsences([]Absence{{From: "2026-11-01", To: "2026-11-02"}, {From: "2026-01-01", To: "2026-01-05"}, {From: "2026-10-20", To: "2026-10-25"}}, "2026-10-25")
	if len(cur) != 2 || cur[0].From != "2026-10-20" {
		t.Fatalf("aktuelle Abwesenheiten: %+v", cur)
	}
	if _, ok := parseDay("2026-02-30"); ok {
		t.Fatal("ungültiges Datum akzeptiert")
	}
}
