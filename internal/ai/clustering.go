package ai

import (
	"math"
	"sort"
	"strings"

	"gonum.org/v1/gonum/stat"
)

// LogCluster represents a group of similar log messages
type LogCluster struct {
	Centroid    string
	Messages    []string
	Frequency   int
	FirstSeen   time.Time
	LastSeen    time.Time
	Severity    string
	Confidence  float64
}

// TFIDFVectorizer converts text into TF-IDF vectors
type TFIDFVectorizer struct {
	vocabulary map[string]int
	idf        map[string]float64
	documents  []string
}

func NewTFIDFVectorizer() *TFIDFVectorizer {
	return &TFIDFVectorizer{
		vocabulary: make(map[string]int),
		idf:        make(map[string]float64),
	}
}

func (v *TFIDFVectorizer) Fit(documents []string) {
	// Build vocabulary
	wordDocs := make(map[string]int)
	for _, doc := range documents {
		words := tokenize(doc)
		seenWords := make(map[string]bool)
		
		for _, word := range words {
			if !seenWords[word] {
				wordDocs[word]++
				seenWords[word] = true
			}
			v.vocabulary[word] = len(v.vocabulary)
		}
	}

	// Calculate IDF
	numDocs := float64(len(documents))
	for word, docCount := range wordDocs {
		v.idf[word] = math.Log(numDocs / float64(docCount))
	}

	v.documents = documents
}

func (v *TFIDFVectorizer) Transform(text string) []float64 {
	vector := make([]float64, len(v.vocabulary))
	words := tokenize(text)
	
	// Calculate term frequency
	tf := make(map[string]float64)
	for _, word := range words {
		tf[word]++
	}

	// Calculate TF-IDF
	for word, freq := range tf {
		if idx, exists := v.vocabulary[word]; exists {
			vector[idx] = freq * v.idf[word]
		}
	}

	return vector
}

// DBSCAN clustering implementation for log messages
type DBSCAN struct {
	Eps       float64
	MinPoints int
}

func NewDBSCAN(eps float64, minPoints int) *DBSCAN {
	return &DBSCAN{
		Eps:       eps,
		MinPoints: minPoints,
	}
}

func (d *DBSCAN) Fit(vectors [][]float64) []int {
	n := len(vectors)
	labels := make([]int, n)
	for i := range labels {
		labels[i] = -1 // Unvisited
	}

	clusterID := 0
	for i := 0; i < n; i++ {
		if labels[i] != -1 {
			continue
		}

		neighbors := d.regionQuery(vectors, i, vectors[i])
		if len(neighbors) < d.MinPoints {
			labels[i] = 0 // Noise
			continue
		}

		clusterID++
		labels[i] = clusterID
		
		// Expand cluster
		seedSet := neighbors
		for len(seedSet) > 0 {
			currentPoint := seedSet[0]
			seedSet = seedSet[1:]

			if labels[currentPoint] == 0 || labels[currentPoint] == -1 {
				if labels[currentPoint] == -1 {
					newNeighbors := d.regionQuery(vectors, currentPoint, vectors[currentPoint])
					if len(newNeighbors) >= d.MinPoints {
						seedSet = append(seedSet, newNeighbors...)
					}
				}
				labels[currentPoint] = clusterID
			}
		}
	}

	return labels
}

func (d *DBSCAN) regionQuery(vectors [][]float64, pointIdx int, point []float64) []int {
	neighbors := make([]int, 0)
	for i, vector := range vectors {
		if cosineDistance(point, vector) <= d.Eps {
			neighbors = append(neighbors, i)
		}
	}
	return neighbors
}

// Helper functions
func cosineDistance(a, b []float64) float64 {
	dotProduct := 0.0
	normA := 0.0
	normB := 0.0

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 1.0
	}

	similarity := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
	return 1.0 - similarity
}

func tokenize(text string) []string {
	// Simple tokenization - split on non-alphanumeric characters
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	// Convert to lowercase and filter stop words
	filtered := make([]string, 0)
	for _, word := range words {
		word = strings.ToLower(word)
		if len(word) > 2 && !isStopWord(word) {
			filtered = append(filtered, word)
		}
	}

	return filtered
}

func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "is": true, "at": true, "which": true, "on": true,
		"and": true, "a": true, "in": true, "or": true, "an": true,
		"for": true, "to": true, "of": true, "with": true, "by": true,
	}
	return stopWords[word]
}
