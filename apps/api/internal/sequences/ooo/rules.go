package ooo

import (
	"regexp"
	"strings"
)

// Rules is the stage-1 regex rule set (ADR-004 rev 2 §5). It is tuned for
// PRECISION, not recall: a firing OOO verdict is right ~95%+ of the time, and
// anything weaker is reported as ambiguous so the Haiku stage (or, with no LLM
// wired, the conservative reply default) decides. Recall on the borderline tail
// is intentionally ceded to stage 2 (§Q1).
//
// The single shared instance compiledRules is immutable after package init and
// safe for concurrent use.
type Rules struct {
	// subjectMarkers match the Subject of mailbox auto-responders. A hit here is
	// a strong OOO signal on its own — autoresponders almost always prefix the
	// subject ("Automatic reply:", "Réponse automatique :", "Out of Office").
	subjectMarkers []*regexp.Regexp
	// strongBody phrases are unambiguous OOO statements in the body snippet
	// ("I am out of the office", "je suis actuellement absent", "en congés").
	strongBody []*regexp.Regexp
	// genericAuto phrases mark an automated message without naming an absence
	// ("do not reply to this message", "ceci est un message automatique"). On
	// their own they are a MEDIUM signal — many transactional autoresponders are
	// not OOO — so one alone is ambiguous.
	genericAuto []*regexp.Regexp
	// returnPhrase marks the presence of a "back on / de retour le …" clause. A
	// MEDIUM signal that pairs with genericAuto to confirm OOO, and feeds the
	// date parser.
	returnPhrase []*regexp.Regexp
}

// verdict is the rules outcome for one reply.
type verdict int

const (
	verdictReply     verdict = iota // confident genuine reply → reply_received
	verdictOOO                      // confident auto-responder → ooo_detected
	verdictAmbiguous                // weak/conflicting signals → escalate to Haiku
)

// ruleResult is the rules verdict plus the matched signal names (for audit /
// debugging) and a confidence estimate.
type ruleResult struct {
	verdict    verdict
	confidence Confidence
	signals    []string
}

// mustCompile compiles each (?i) pattern, panicking at init on a bad pattern —
// a malformed rule is a programming error that must fail loudly at startup.
func mustCompile(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(p)
	}
	return out
}

// compiledRules is the frozen v1 rule set. Patterns are case-insensitive; French
// is written with explicit accents AND an accent-stripped fallback is applied to
// the input (foldAccents) so "congés"/"conges" and "déplacement"/"deplacement"
// both match without doubling every pattern.
var compiledRules = &Rules{
	subjectMarkers: mustCompile(
		`(?i)\bout[\s-]?of[\s-]?office\b`,
		`(?i)\bo\.?o\.?o\.?\b`,
		`(?i)\bauto(?:matic|mated)?[\s-]?(?:reply|response|reponse)\b`,
		`(?i)reponse\s+automatique`,
		`(?i)\babsence\b`,
		`(?i)\bon\s+(?:vacation|holiday|leave|annual\s+leave|pto)\b`,
		`(?i)\bconges?\b`,
		`(?i)\bvacances\b`,
	),
	strongBody: mustCompile(
		// English
		`(?i)\bout\s+of\s+(?:the\s+)?office\b`,
		`(?i)\bout[\s-]?of[\s-]?office\b`,
		`(?i)\b(?:i\s*(?:'?m|\s+am)|currently)\s+(?:out\s+of\s+the\s+office|away|on\s+leave|on\s+(?:vacation|holiday|pto))\b`,
		`(?i)\baway\s+from\s+(?:my|the)\s+(?:office|desk|e?-?mail)\b`,
		`(?i)\bon\s+(?:annual|parental|maternity|paternity|sick|medical)\s+leave\b`,
		`(?i)\bi\s+will\s+be\s+(?:back|returning|out\s+of\s+(?:the\s+)?office)\b`,
		`(?i)\bback\s+in\s+the\s+office\b`,
		`(?i)\blimited\s+access\s+to\s+(?:my\s+)?e?-?mail\b`,
		`(?i)\b(?:will|i'?ll)\s+respond\s+(?:to\s+your\s+e?-?mail\s+)?(?:up)?on\s+my\s+return\b`,
		// French (accents stripped on input by foldAccents)
		`(?i)\b(?:je\s+suis\s+)?(?:actuellement\s+)?absent(?:e)?\b`,
		`(?i)\babsence\s+du\s+bureau\b`,
		`(?i)\ben\s+conges?\b`,
		`(?i)\ben\s+vacances\b`,
		`(?i)\ben\s+deplacement\b`,
		`(?i)\bje\s+serai\s+de\s+retour\b`,
		`(?i)\bde\s+retour\s+(?:au\s+bureau|le|au)\b`,
		`(?i)\bindisponible\s+jusqu`,
		`(?i)\bacces\s+limite\s+a\s+mes?\b`,
		`(?i)\bdes\s+mon\s+retour\b`,
	),
	genericAuto: mustCompile(
		`(?i)\bdo\s+not\s+reply\s+to\s+this\b`,
		`(?i)\bthis\s+is\s+an?\s+automat(?:ed|ic)\s+(?:reply|response|message|e?-?mail)\b`,
		`(?i)\bne\s+(?:pas\s+)?repondre\s+a\s+ce(?:t)?\b`,
		`(?i)\b(?:ceci\s+est|message\s+genere\s+automatiquement|message\s+automatique)\b`,
	),
	returnPhrase: mustCompile(
		`(?i)\bback\s+on\b`,
		`(?i)\breturn(?:ing)?\s+(?:on|the)\b`,
		`(?i)\bde\s+retour\s+le\b`,
		`(?i)\bjusqu['’ ]?au\b`,
		`(?i)\ba\s+partir\s+du\b`,
		`(?i)\buntil\b`,
	),
}

// Evaluate classifies one reply by its accumulated signals (ADR-004 rev 2 §5).
//
// Scoring, biased toward precision:
//   - A subject auto-reply marker OR any strong body OOO phrase ⇒ confident OOO.
//   - Otherwise, two or more medium signals (genericAuto + returnPhrase) ⇒ OOO.
//   - Exactly one medium signal ⇒ ambiguous (escalate; an isolated "automated
//     message" or a bare "until Friday" is too weak to call).
//   - No signals ⇒ confident reply.
func (r *Rules) Evaluate(body ReplyBody) ruleResult {
	subject := foldAccents(strings.ToLower(body.Subject))
	text := foldAccents(strings.ToLower(body.Subject + "\n" + body.Body))

	var signals []string

	strong := false
	if name, ok := firstMatch(r.subjectMarkers, subject); ok {
		strong = true
		signals = append(signals, "subject:"+name)
	}
	if name, ok := firstMatch(r.strongBody, text); ok {
		strong = true
		signals = append(signals, "body:"+name)
	}

	medium := 0
	if _, ok := firstMatch(r.genericAuto, text); ok {
		medium++
		signals = append(signals, "auto")
	}
	if _, ok := firstMatch(r.returnPhrase, text); ok {
		medium++
		signals = append(signals, "return")
	}

	switch {
	case strong:
		return ruleResult{verdict: verdictOOO, confidence: 0.97, signals: signals}
	case medium >= 2:
		return ruleResult{verdict: verdictOOO, confidence: 0.90, signals: signals}
	case medium == 1:
		return ruleResult{verdict: verdictAmbiguous, confidence: ambiguousReplyConfidence, signals: signals}
	default:
		return ruleResult{verdict: verdictReply, confidence: 0.96, signals: signals}
	}
}

// firstMatch returns the (truncated) source of the first matching pattern.
func firstMatch(patterns []*regexp.Regexp, s string) (string, bool) {
	for _, re := range patterns {
		if re.MatchString(s) {
			src := re.String()
			if len(src) > 28 {
				src = src[:28]
			}
			return src, true
		}
	}
	return "", false
}

// foldAccents strips the common French diacritics so a single accent-bearing
// pattern matches both spellings. It is a deliberately small, allocation-light
// fold over the Latin-1 accented letters that appear in OOO French — not a full
// Unicode NFD normaliser (the rules need only é/è/ê/à/ç/ù/î/ô etc.).
func foldAccents(s string) string {
	if !strings.ContainsAny(s, "àâäáãéèêëíìîïóòôöõúùûüçñ") {
		return s
	}
	return accentFolder.Replace(s)
}

var accentFolder = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)
