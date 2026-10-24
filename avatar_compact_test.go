package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func avatarWithPaint(v int, paint string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"v":%d,"skin":"#a7a9ac","hair":"#6d6f73","eyes":"#3d3f42","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"short","eyeStyle":"classic","browStyle":"normal","mouthStyle":"smile","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"none","extras":[],"texture":true,"paint":%s}`, v, paint))
}

func TestAvatarPaintCompact(t *testing.T) {
	// ganze Seite rot, ein Pixel blau → "*" + 1 Ausnahme
	px := []string{}
	for i := 0; i < 256; i++ {
		c := "#ff0000"
		if i == 17 {
			c = "#0000ff"
		}
		px = append(px, fmt.Sprintf(`"%d":"%s"`, i, c))
	}
	full := `{"base":{"front":{` + strings.Join(px, ",") + `}}}`
	out, err := validateAvatar(avatarWithPaint(2, full))
	if err != nil {
		t.Fatal(err)
	}
	var a Avatar
	_ = json.Unmarshal([]byte(out), &a)
	f := a.Paint["base"]["front"]
	if len(f) != 2 || f["*"] != "#ff0000" || f["17"] != "#0000ff" {
		t.Fatalf("nicht verdichtet: %v", f)
	}
	if len(out) > 1000 {
		t.Fatalf("Ergebnis zu groß: %d Bytes", len(out))
	}
	// "*" + überflüssige gleiche Pixel → nur "*"
	out, err = validateAvatar(avatarWithPaint(2, `{"over":{"top":{"*":"","3":"","4":"#00ff00"}}}`))
	if err != nil || !strings.Contains(out, `"top":{"*":"","4":"#00ff00"}`) {
		t.Fatalf("Redundanz nicht entfernt: %s %v", out, err)
	}
	// unvollständige Seite bleibt pixelweise
	out, _ = validateAvatar(avatarWithPaint(2, `{"base":{"left":{"1":"#ff0000","2":"#ff0000"}}}`))
	if strings.Contains(out, `"*"`) {
		t.Fatalf("unvollständige Seite verdichtet: %s", out)
	}
	// ungültig: "*" in v1, durchsichtig auf dem Kopf
	for _, bad := range []json.RawMessage{avatarWithPaint(1, `{"base":{"front":{"*":"#ff0000"}}}`), avatarWithPaint(2, `{"base":{"front":{"*":""}}}`)} {
		if _, err := validateAvatar(bad); err == nil {
			t.Fatalf("akzeptiert: %s", bad)
		}
	}
}
