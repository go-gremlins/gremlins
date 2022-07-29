package testmodule

import "testing"

func TestModule(t *testing.T) {
	got := getData()

	if got != "ok" {
		t.Error("it should be ok!")
	}
}
