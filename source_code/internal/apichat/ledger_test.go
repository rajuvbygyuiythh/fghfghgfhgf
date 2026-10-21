package apichat

import "testing"

func TestLedgerRemembersAnsweredFacts(t *testing.T) {
	d := &Director{Turn: 3, UserAge: 21, UserGender: "f", UserPlace: "texas"}
	d.syncLedger()

	if !d.Ledger.Known(FactAge) || !d.Ledger.Known(FactGender) || !d.Ledger.Known(FactPlace) {
		t.Fatalf("facts not recorded: %+v", d.Ledger)
	}
	if d.Ledger.ShouldAsk(FactAge, 4, 3) {
		t.Fatal("a known age must never be asked again")
	}
	if !d.Ledger.ShouldAsk(FactName, 4, 3) {
		t.Fatal("the name is still unknown and can be asked")
	}
}

func TestLedgerBlocksImmediateReAskButAllowsLater(t *testing.T) {
	l := Ledger{}
	l.MarkAsked(FactName, 5)
	if l.ShouldAsk(FactName, 6, 3) {
		t.Fatal("re-asking within the window must be blocked")
	}
	if !l.ShouldAsk(FactName, 9, 3) {
		t.Fatal("outside the window the question is allowed again")
	}
}

func TestAskedFactsReadsQuestionKinds(t *testing.T) {
	cases := map[string]FactKey{
		"how old r u?":  FactAge,
		"m or f?":       FactGender,
		"where u from?": FactPlace,
		"whats ur name": FactName,
		"wyd?":          FactDoing,
		"whats ur snap": FactSnap,
	}
	for line, want := range cases {
		found := false
		for _, fact := range askedFacts(line) {
			if fact == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q did not produce %s (got %v)", line, want, askedFacts(line))
		}
	}
}

func TestFactAnsweredUsesDirectorFields(t *testing.T) {
	d := &Director{UserAge: 22, UserName: "sam"}
	if !d.factAnswered(FactAge) || !d.factAnswered(FactName) {
		t.Fatal("answered facts not detected")
	}
	if d.factAnswered(FactDoing) {
		t.Fatal("doing is still unknown")
	}
}
