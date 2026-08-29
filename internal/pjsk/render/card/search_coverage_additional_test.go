package card

import (
	"errors"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestSearchServiceStableErrorAndAllowUnreleasedBranches(t *testing.T) {
	var nilSearcher *SearchService
	if nilSearcher.WithAllowUnreleased(true) != nil {
		t.Fatal("nil search service produced a clone")
	}

	now := time.Now().UnixMilli()
	future := &masterdata.Card{ID: 999, CharacterID: 5, CardRarityType: "rarity_4", ReleaseAt: now + 60_000}
	futureSource := &lookupTestSource{card: future}
	searcher := NewSearchService(futureSource, NewParser(defaultNicknames))
	if _, err := searcher.Search(""); err == nil {
		t.Fatal("empty single-card query succeeded")
	}
	if _, err := searcher.Search("998"); err == nil {
		t.Fatal("missing ID query succeeded")
	}
	card, err := searcher.WithAllowUnreleased(true).Search("999")
	if err != nil || card == nil || card.ID != 999 {
		t.Fatalf("allow-unreleased ID query = %+v, %v", card, err)
	}

	filterErr := errors.New("filter unavailable")
	errorSource := &lookupTestSource{filterFunc: func(*PjskCardQueryInfo) ([]*masterdata.Card, error) {
		return nil, filterErr
	}}
	errorSearcher := NewSearchService(errorSource, NewParser(defaultNicknames))
	for _, query := range []string{"mnr-1", "-1", "mnr 4星"} {
		if _, err := errorSearcher.Search(query); !errors.Is(err, filterErr) {
			t.Errorf("Search(%q) error = %v, want filter error", query, err)
		}
	}
	for _, query := range []string{"mnr 4星", "998", "mnr-1", "-1"} {
		if _, err := errorSearcher.SearchList(query); err == nil {
			t.Errorf("SearchList(%q) unexpectedly succeeded", query)
		}
	}
	if _, err := errorSearcher.SearchList(""); err == nil {
		t.Fatal("empty list query succeeded")
	}

	emptySource := &lookupTestSource{filterFunc: func(*PjskCardQueryInfo) ([]*masterdata.Card, error) {
		return nil, nil
	}}
	emptySearcher := NewSearchService(emptySource, NewParser(defaultNicknames))
	if _, err := emptySearcher.Search("mnr 4星"); err == nil {
		t.Fatal("empty filter single-card query succeeded")
	}
	if _, err := emptySearcher.SearchList("mnr 4星"); err == nil {
		t.Fatal("empty filter list query succeeded")
	}

	futureFilter := &lookupTestSource{filterFunc: func(*PjskCardQueryInfo) ([]*masterdata.Card, error) {
		return []*masterdata.Card{future}, nil
	}}
	if _, err := NewSearchService(futureFilter, NewParser(defaultNicknames)).Search("mnr 4星"); err == nil {
		t.Fatal("future-only filter single-card query succeeded")
	}
}

func TestSearchServiceSequenceHelpersBoundaryBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	var nilSearcher *SearchService
	if _, err := nilSearcher.cardByCharacterAndSeq(5, 1, now); err == nil {
		t.Fatal("nil character searcher succeeded")
	}
	if _, err := nilSearcher.latestCard(-1, now); err == nil {
		t.Fatal("nil latest searcher succeeded")
	}

	released := []*masterdata.Card{
		{ID: 1, CharacterID: 5, ReleaseAt: now - 2000},
		{ID: 2, CharacterID: 5, ReleaseAt: now - 1000},
	}
	source := &lookupTestSource{filterFunc: func(info *PjskCardQueryInfo) ([]*masterdata.Card, error) {
		if info.CharacterID != 0 && info.CharacterID != 5 {
			return nil, nil
		}
		return append([]*masterdata.Card(nil), released...), nil
	}}
	searcher := NewSearchService(source, NewParser(defaultNicknames))
	if _, err := searcher.cardByCharacterAndSeq(0, 1, now); err == nil {
		t.Fatal("zero character ID succeeded")
	}
	if _, err := searcher.cardByCharacterAndSeq(5, 0, now); err == nil {
		t.Fatal("zero character sequence succeeded")
	}
	card, err := searcher.cardByCharacterAndSeq(5, 1, now)
	if err != nil || card.ID != 1 {
		t.Fatalf("positive sequence = %+v, %v", card, err)
	}
	for _, sequence := range []int{3, -3} {
		if _, err := searcher.cardByCharacterAndSeq(5, sequence, now); err == nil {
			t.Errorf("out-of-range character sequence %d succeeded", sequence)
		}
	}
	if _, err := searcher.latestCard(0, now); err == nil {
		t.Fatal("non-negative latest sequence succeeded")
	}
	if _, err := searcher.latestCard(-3, now); err == nil {
		t.Fatal("out-of-range latest sequence succeeded")
	}

	filterErr := errors.New("filter unavailable")
	errorSearcher := NewSearchService(&lookupTestSource{filterFunc: func(*PjskCardQueryInfo) ([]*masterdata.Card, error) {
		return nil, filterErr
	}}, NewParser(defaultNicknames))
	if _, err := errorSearcher.cardByCharacterAndSeq(5, 1, now); !errors.Is(err, filterErr) {
		t.Fatalf("character filter error = %v", err)
	}
	if _, err := errorSearcher.latestCard(-1, now); !errors.Is(err, filterErr) {
		t.Fatalf("latest filter error = %v", err)
	}

	emptySearcher := NewSearchService(&lookupTestSource{filterFunc: func(*PjskCardQueryInfo) ([]*masterdata.Card, error) {
		return nil, nil
	}}, NewParser(defaultNicknames))
	if _, err := emptySearcher.cardByCharacterAndSeq(5, 1, now); err == nil {
		t.Fatal("empty character sequence succeeded")
	}
	if _, err := emptySearcher.latestCard(-1, now); err == nil {
		t.Fatal("empty latest sequence succeeded")
	}

	future := &masterdata.Card{ID: 3, CharacterID: 5, ReleaseAt: now + 60_000}
	futureSearcher := NewSearchService(&lookupTestSource{filterFunc: func(*PjskCardQueryInfo) ([]*masterdata.Card, error) {
		return []*masterdata.Card{future}, nil
	}}, NewParser(defaultNicknames))
	if _, err := futureSearcher.cardByCharacterAndSeq(5, 1, now); err == nil {
		t.Fatal("future-only character sequence succeeded")
	}
	if _, err := futureSearcher.latestCard(-1, now); err == nil {
		t.Fatal("future-only latest sequence succeeded")
	}
}
