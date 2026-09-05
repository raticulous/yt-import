package matcher

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// LevenshteinDistance computes the Levenshtein distance between two rune slices.
func LevenshteinDistance(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	dp := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		dp[j] = j
	}

	for i := 1; i <= la; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= lb; j++ {
			temp := dp[j]
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dp[j] = min(dp[j]+1, min(dp[j-1]+1, prev+cost))
			prev = temp
		}
	}
	return dp[lb]
}

// LevenshteinSimilarity returns a normalized similarity between 0.0 and 1.0.
func LevenshteinSimilarity(s1, s2 string) float64 {
	r1, r2 := []rune(s1), []rune(s2)
	maxLen := max(len(r1), len(r2))
	if maxLen == 0 {
		return 1.0
	}
	dist := LevenshteinDistance(r1, r2)
	return 1.0 - (float64(dist) / float64(maxLen))
}

// JaroWinkler returns the Jaro-Winkler similarity between two strings (0.0 to 1.0).
func JaroWinkler(s1, s2 string) float64 {
	r1, r2 := []rune(s1), []rune(s2)
	l1, l2 := len(r1), len(r2)
	if l1 == 0 && l2 == 0 {
		return 1.0
	}
	if l1 == 0 || l2 == 0 {
		return 0.0
	}

	matchDistance := max(l1, l2)/2 - 1
	if matchDistance < 0 {
		matchDistance = 0
	}

	s1Matches := make([]bool, l1)
	s2Matches := make([]bool, l2)
	matches := 0
	transpositions := 0

	for i := 0; i < l1; i++ {
		start := max(0, i-matchDistance)
		end := min(i+matchDistance+1, l2)
		for j := start; j < end; j++ {
			if s2Matches[j] || r1[i] != r2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	k := 0
	for i := 0; i < l1; i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if r1[i] != r2[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	jaro := (m/float64(l1) + m/float64(l2) + (m-float64(transpositions/2))/m) / 3.0

	// Winkler prefix adjustment (up to 4 characters)
	prefix := 0
	for i := 0; i < min(min(l1, l2), 4); i++ {
		if r1[i] == r2[i] {
			prefix++
		} else {
			break
		}
	}
	return jaro + float64(prefix)*0.1*(1.0-jaro)
}

// TokenSetRatio calculates the similarity based on intersection and difference of words.
func TokenSetRatio(s1, s2 string) float64 {
	tokens1 := tokenize(s1)
	tokens2 := tokenize(s2)

	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, t := range tokens1 {
		set1[t] = true
	}
	set2 := make(map[string]bool)
	for _, t := range tokens2 {
		set2[t] = true
	}

	var intersection, diff1, diff2 []string
	for t := range set1 {
		if set2[t] {
			intersection = append(intersection, t)
		} else {
			diff1 = append(diff1, t)
		}
	}
	for t := range set2 {
		if !set1[t] {
			diff2 = append(diff2, t)
		}
	}

	sort.Strings(intersection)
	sort.Strings(diff1)
	sort.Strings(diff2)

	interStr := strings.Join(intersection, " ")
	t0 := strings.TrimSpace(interStr)
	t1 := strings.TrimSpace(interStr + " " + strings.Join(diff1, " "))
	t2 := strings.TrimSpace(interStr + " " + strings.Join(diff2, " "))

	score1 := LevenshteinSimilarity(t0, t1)
	score2 := LevenshteinSimilarity(t0, t2)
	score3 := LevenshteinSimilarity(t1, t2)

	return math.Max(score1, math.Max(score2, score3))
}

func tokenize(s string) []string {
	var words []string
	var cur strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(unicode.ToLower(r))
		} else if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}
