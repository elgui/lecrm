package ooo

import (
	"encoding/json"
	"os"
	"testing"
)

// fixtureSample is one frozen, anonymised reply from testdata/fixtures.json
// (ADR-004 rev 2 §5). Expect is the labelled rules verdict:
//
//	"ooo"       — rules-certain out-of-office auto-responder
//	"reply"     — rules-certain genuine human reply
//	"ambiguous" — rules cannot call; must fall through to the Haiku stage
type fixtureSample struct {
	ID      string `json:"id"`
	Lang    string `json:"lang"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Expect  string `json:"expect"`
}

type fixtureFile struct {
	Samples []fixtureSample `json:"samples"`
}

func loadFixtures(t *testing.T) []fixtureSample {
	t.Helper()
	raw, err := os.ReadFile("testdata/fixtures.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var f fixtureFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	if len(f.Samples) == 0 {
		t.Fatal("fixtures.json contains no samples")
	}
	return f.Samples
}

func ruleVerdictName(v verdict) string {
	switch v {
	case verdictOOO:
		return "ooo"
	case verdictReply:
		return "reply"
	default:
		return "ambiguous"
	}
}

// TestRulesPrecision is the ADR-004 rev 2 §5 acceptance gate: the stage-1 regex
// rule set must classify out-of-office replies at ~95% precision (Q1's
// reconsideration trigger is "measured OOO false-positive rate >5%"). It runs
// the frozen ~120-sample fixture set through the live rules and asserts the OOO
// precision floor, plus a recall floor (so a rule set that never fires can't pass)
// and a hard ceiling on the costliest error: a genuine reply misread as OOO.
func TestRulesPrecision(t *testing.T) {
	samples := loadFixtures(t)

	// confusion[expectedLabel][predictedVerdict]
	confusion := map[string]map[string]int{}
	count := map[string]int{}
	for _, s := range samples {
		res := compiledRules.Evaluate(ReplyBody{From: "x@example.test", Subject: s.Subject, Body: s.Body})
		pred := ruleVerdictName(res.verdict)
		count[s.Expect]++
		if confusion[s.Expect] == nil {
			confusion[s.Expect] = map[string]int{}
		}
		confusion[s.Expect][pred]++
		if pred != s.Expect {
			t.Logf("mismatch %-12s expect=%-9s pred=%-9s | subj=%q body=%q", s.ID, s.Expect, pred, s.Subject, s.Body)
		}
	}
	for _, exp := range []string{"ooo", "reply", "ambiguous"} {
		t.Logf("expect=%-9s n=%-3d -> %v", exp, count[exp], confusion[exp])
	}

	// Precision of the OOO verdict: of everything the rules call OOO, the share
	// that is truly OOO. A false positive is a reply OR an ambiguous sample the
	// rules wrongly fire OOO on.
	tp := confusion["ooo"]["ooo"]
	fpReply := confusion["reply"]["ooo"]
	fpAmbiguous := confusion["ambiguous"]["ooo"]
	predictedOOO := tp + fpReply + fpAmbiguous

	if predictedOOO == 0 {
		t.Fatal("rules never fired an OOO verdict on any fixture — precision is undefined and the rule set is inert")
	}
	precision := float64(tp) / float64(predictedOOO)
	t.Logf("OOO precision = %d/%d = %.4f (target ≥ 0.95)", tp, predictedOOO, precision)
	if precision < 0.95 {
		t.Errorf("OOO precision %.4f below the ADR-004 rev 2 §5 target of 0.95 (false positives: %d reply, %d ambiguous)", precision, fpReply, fpAmbiguous)
	}

	// Recall floor: the rules must actually catch the bulk of labelled OOO replies,
	// otherwise a do-nothing rule set would trivially clear the precision gate.
	if oooTotal := count["ooo"]; oooTotal > 0 {
		recall := float64(tp) / float64(oooTotal)
		t.Logf("OOO recall = %d/%d = %.4f (floor ≥ 0.90)", tp, oooTotal, recall)
		if recall < 0.90 {
			t.Errorf("OOO recall %.4f below the 0.90 floor — rules miss too many labelled OOO replies", recall)
		}
	}

	// Costliest error: a genuine human reply misread as OOO pauses a live
	// conversation and reschedules a contact who actually engaged. Q1 caps the
	// false-positive rate at 5%; hold genuine-reply false OOO to that bound.
	if replyTotal := count["reply"]; replyTotal > 0 {
		fpRate := float64(fpReply) / float64(replyTotal)
		t.Logf("genuine-reply→OOO rate = %d/%d = %.4f (ceiling ≤ 0.05)", fpReply, replyTotal, fpRate)
		if fpRate > 0.05 {
			t.Errorf("genuine-reply→OOO false-positive rate %.4f exceeds the 0.05 ceiling (Q1 reconsideration trigger)", fpRate)
		}
	}
}

// TestFixturesShape guards the fixture corpus itself: it must stay broad enough
// (FR + EN, all three labels) to make the precision gate meaningful. A shrunk or
// single-language corpus would silently weaken TestRulesPrecision.
func TestFixturesShape(t *testing.T) {
	samples := loadFixtures(t)

	byLabel := map[string]int{}
	byLang := map[string]int{}
	for _, s := range samples {
		byLabel[s.Expect]++
		byLang[s.Lang]++
		switch s.Expect {
		case "ooo", "reply", "ambiguous":
		default:
			t.Errorf("fixture %s has unknown expect=%q (want ooo|reply|ambiguous)", s.ID, s.Expect)
		}
	}
	if len(samples) < 100 {
		t.Errorf("fixture corpus shrank to %d samples; ADR-004 rev 2 §5 specifies ~120", len(samples))
	}
	for _, label := range []string{"ooo", "reply", "ambiguous"} {
		if byLabel[label] == 0 {
			t.Errorf("fixture corpus has no %q samples", label)
		}
	}
	for _, lang := range []string{"fr", "en"} {
		if byLang[lang] == 0 {
			t.Errorf("fixture corpus has no %q samples (must cover FR + EN)", lang)
		}
	}
}
