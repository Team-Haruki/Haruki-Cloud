package mysekai

import "testing"

func TestResolveNicknameCharacterIDSupportsCompactAliases(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{query: "akt", want: 11},
		{query: "khn", want: 9},
		{query: "tks", want: 13},
		{query: "enana", want: 19},
		{query: "mei", want: 25},
		{query: "kai", want: 26},
		{query: "天馬司", want: 13},
		{query: "小豆沢こはね", want: 9},
	}

	for _, tc := range cases {
		got, ok := ResolveNicknameCharacterID(tc.query)
		if !ok {
			t.Fatalf("ResolveNicknameCharacterID(%q) did not resolve", tc.query)
		}
		if got != tc.want {
			t.Fatalf("ResolveNicknameCharacterID(%q) = %d, want %d", tc.query, got, tc.want)
		}
	}
}
