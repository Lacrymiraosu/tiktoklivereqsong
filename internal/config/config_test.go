package config

import (
	"errors"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if c.MasterVolume != 10 {
		t.Errorf("default master volume = %d, want 10", c.MasterVolume)
	}
}

func TestValidationProblemsHaveCodes(t *testing.T) {
	c := Default()
	c.Connection.DashboardPort = 80
	c.Command.SkipCmd = "skip"
	err := c.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 2 {
		t.Fatalf("want 2 problems, got %v", err)
	}
	if code, _ := ve.ErrCode(); code != "port_range" {
		t.Errorf("first code = %q", code)
	}
	if p := ve.Problems[1]; p.Code != "mod_cmd" || p.Vars["cmd"] != "skip" {
		t.Errorf("second problem = %+v", p)
	}
}

func TestPerModeStars(t *testing.T) {
	c := Default()
	c.Rules.MinStars, c.Rules.MaxStars = 2, 6
	c.Rules.ModeStars = map[string]StarRange{"mania": {Max: 4}, "taiko": {Min: 3}}
	if min, max, per := c.Rules.StarLimits("mania"); min != 2 || max != 4 || !per {
		t.Errorf("mania = %v %v %v", min, max, per)
	}
	if min, max, per := c.Rules.StarLimits("osu"); min != 2 || max != 6 || per {
		t.Errorf("osu = %v %v %v", min, max, per)
	}
	c.Rules.ModeStars["fruits"] = StarRange{Min: 7} // 7 > global max 6
	c.Rules.ModeStars["bogus"] = StarRange{}
	var ve *ValidationError
	if err := c.Validate(); !errors.As(err, &ve) || len(ve.Problems) != 2 {
		t.Fatalf("want 2 problems, got %v", err)
	}
}
