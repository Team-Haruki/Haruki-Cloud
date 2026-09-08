package music

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func referenceEditDistance(left, right []rune) int {
	matrix := make([][]int, len(left)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(right)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}
	for i, a := range left {
		for j, b := range right {
			cost := 0
			if a != b {
				cost = 1
			}
			matrix[i+1][j+1] = min(matrix[i][j+1]+1, matrix[i+1][j]+1, matrix[i][j]+cost)
		}
	}
	return matrix[len(left)][len(right)]
}

func TestBoundedEditDistanceMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(71, 29))
	alphabet := []rune("abc歌曲é")
	sample := func(n int) []rune {
		out := make([]rune, n)
		for i := range out {
			out[i] = alphabet[rng.IntN(len(alphabet))]
		}
		return out
	}
	for range 2500 {
		left, right := sample(rng.IntN(90)), sample(rng.IntN(90))
		expected := referenceEditDistance(left, right)
		for limit := range 4 {
			if got, want := boundedLevenshteinDistance(left, right, limit), min(expected, limit+1); got != want {
				t.Fatalf("distance(%q,%q,%d)=%d want %d", string(left), string(right), limit, got, want)
			}
		}
	}
	for _, left := range []string{"", "歌曲", "abcdefgh", strings.Repeat("x", 80)} {
		for _, right := range []string{left, left + "a", "a" + left, left + "abc"} {
			for limit := range 4 {
				want := min(referenceEditDistance([]rune(left), []rune(right)), limit+1)
				if got := boundedLevenshteinDistance([]rune(left), []rune(right), limit); got != want {
					t.Fatalf("near match %q/%q limit=%d got=%d want=%d", left, right, limit, got, want)
				}
			}
		}
	}
}

func TestFuzzySubstringMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(93, 18))
	for range 700 {
		query := make([]rune, 4+rng.IntN(10))
		candidate := make([]rune, len(query)+1+rng.IntN(12))
		for i := range query {
			query[i] = rune('a' + rng.IntN(3))
		}
		for i := range candidate {
			candidate[i] = rune('a' + rng.IntN(3))
		}
		var want musicFuzzyScore
		found := false
		limit := fuzzyDistanceLimit(len(query))
		for length := max(1, len(query)-limit); length <= min(len(candidate), len(query)+limit); length++ {
			for start := 0; start+length <= len(candidate); start++ {
				d := referenceEditDistance(query, candidate[start:start+length])
				if d > limit {
					continue
				}
				score := musicFuzzyScore{matchType: 2, distance: d, lengthGap: absInt(length - len(query))}
				if !found || compareMusicFuzzyScore(score, want) < 0 {
					want = score
					found = true
				}
			}
		}
		got, ok := scoreMusicFuzzySubstring(query, candidate)
		if ok != found || (ok && got != want) {
			t.Fatalf("substring %q/%q got=%+v/%v want=%+v/%v", string(query), string(candidate), got, ok, want, found)
		}
	}
}

func BenchmarkFuzzyCandidateMiss(b *testing.B) {
	for b.Loop() {
		scoreMusicFuzzyCandidate("zzzzzzzz", "abcdefghijklmnopqrstuvwx00000000")
	}
}
