package ai

import (
	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/gonum/stat/distuv"
)

// AnomalyDetector implements various statistical methods for anomaly detection
type AnomalyDetector struct {
	// Configuration parameters
	MinDataPoints    int     // Minimum number of points needed for analysis
	ConfidenceLevel float64 // Statistical confidence level (e.g., 0.95)
	WindowSize      int     // Size of sliding window for local analysis
	SeasonalPeriod  int     // For seasonal patterns (e.g., 24 for hourly data)
}

// TimeSeriesPoint represents a single observation in time
type TimeSeriesPoint struct {
	Timestamp time.Time
	Value     float64
	Metadata  map[string]interface{}
}

// AnomalyResult contains the analysis results for a data point
type AnomalyResult struct {
	IsAnomaly       bool
	Score           float64    // Normalized anomaly score (0-1)
	Probability     float64    // Probability of being normal
	ExpectedRange   Range      // Expected value range
	Method          string     // Detection method used
	Timestamp       time.Time
}

type Range struct {
	Lower float64
	Upper float64
}

func NewAnomalyDetector(config map[string]interface{}) *AnomalyDetector {
	detector := &AnomalyDetector{
		MinDataPoints:    30,
		ConfidenceLevel: 0.95,
		WindowSize:      20,
		SeasonalPeriod:  24,
	}

	// Override defaults with provided config
	if minPoints, ok := config["min_data_points"].(int); ok {
		detector.MinDataPoints = minPoints
	}
	if confidence, ok := config["confidence_level"].(float64); ok {
		detector.ConfidenceLevel = confidence
	}
	if window, ok := config["window_size"].(int); ok {
		detector.WindowSize = window
	}
	if period, ok := config["seasonal_period"].(int); ok {
		detector.SeasonalPeriod = period
	}

	return detector
}

// DetectAnomalies performs ensemble anomaly detection using multiple methods
func (d *AnomalyDetector) DetectAnomalies(points []TimeSeriesPoint) []AnomalyResult {
	if len(points) < d.MinDataPoints {
		return make([]AnomalyResult, len(points))
	}

	results := make([]AnomalyResult, len(points))
	
	// Apply different detection methods
	statisticalResults := d.statisticalDetection(points)
	seasonalResults := d.seasonalDetection(points)
	robustResults := d.robustDetection(points)

	// Combine results using weighted ensemble
	weights := map[string]float64{
		"statistical": 0.4,
		"seasonal":   0.3,
		"robust":     0.3,
	}

	for i := range points {
		results[i] = d.ensembleResults(
			statisticalResults[i],
			seasonalResults[i],
			robustResults[i],
			weights,
			points[i].Timestamp,
		)
	}

	return results
}

// statisticalDetection uses parametric statistical methods
func (d *AnomalyDetector) statisticalDetection(points []TimeSeriesPoint) []AnomalyResult {
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Value
	}

	results := make([]AnomalyResult, len(points))
	
	// Calculate rolling statistics
	for i := range points {
		start := max(0, i-d.WindowSize)
		window := values[start:i+1]
		
		if len(window) < 3 {
			continue
		}

		mean, std := stat.MeanStdDev(window, nil)
		
		// Use Student's t-distribution for small sample sizes
		df := float64(len(window) - 1)
		dist := distuv.StudentsT{Mu: mean, Sigma: std, Nu: df}
		
		value := points[i].Value
		prob := 2 * min(dist.CDF(value), 1-dist.CDF(value)) // Two-tailed test
		
		criticalValue := dist.Quantile(1 - (1-d.ConfidenceLevel)/2)
		
		results[i] = AnomalyResult{
			IsAnomaly:   prob < (1 - d.ConfidenceLevel),
			Score:       math.Abs((value - mean) / std),
			Probability: prob,
			ExpectedRange: Range{
				Lower: mean - criticalValue*std,
				Upper: mean + criticalValue*std,
			},
			Method:    "statistical",
			Timestamp: points[i].Timestamp,
		}
	}

	return results
}

// seasonalDetection handles seasonal patterns in the data
func (d *AnomalyDetector) seasonalDetection(points []TimeSeriesPoint) []AnomalyResult {
	if len(points) < 2*d.SeasonalPeriod {
		return make([]AnomalyResult, len(points))
	}

	results := make([]AnomalyResult, len(points))
	
	// Calculate seasonal components
	seasonal := make([]float64, d.SeasonalPeriod)
	seasonalStd := make([]float64, d.SeasonalPeriod)
	
	for i := 0; i < d.SeasonalPeriod; i++ {
		values := make([]float64, 0)
		for j := i; j < len(points); j += d.SeasonalPeriod {
			values = append(values, points[j].Value)
		}
		
		if len(values) > 0 {
			seasonal[i], seasonalStd[i] = stat.MeanStdDev(values, nil)
		}
	}

	// Detect anomalies using seasonal patterns
	for i, point := range points {
		idx := i % d.SeasonalPeriod
		expected := seasonal[idx]
		stdDev := seasonalStd[idx]
		
		if stdDev == 0 {
			continue
		}

		deviation := math.Abs(point.Value - expected) / stdDev
		prob := 2 * (1 - stat.NormalCDF(deviation, 0, 1))

		results[i] = AnomalyResult{
			IsAnomaly:   deviation > 3, // 3-sigma rule
			Score:       deviation / 3,  // Normalize to 0-1
			Probability: prob,
			ExpectedRange: Range{
				Lower: expected - 3*stdDev,
				Upper: expected + 3*stdDev,
			},
			Method:    "seasonal",
			Timestamp: point.Timestamp,
		}
	}

	return results
}

// robustDetection uses non-parametric methods resistant to outliers
func (d *AnomalyDetector) robustDetection(points []TimeSeriesPoint) []AnomalyResult {
	results := make([]AnomalyResult, len(points))
	
	for i := range points {
		start := max(0, i-d.WindowSize)
		window := make([]float64, i-start+1)
		for j := range window {
			window[j] = points[start+j].Value
		}
		
		if len(window) < 3 {
			continue
		}

		// Calculate median and MAD (Median Absolute Deviation)
		median := stat.Quantile(0.5, stat.Empirical, window, nil)
		deviations := make([]float64, len(window))
		for j := range window {
			deviations[j] = math.Abs(window[j] - median)
		}
		mad := stat.Quantile(0.5, stat.Empirical, deviations, nil) * 1.4826 // Scale factor for normal distribution

		value := points[i].Value
		score := math.Abs(value - median) / mad
		
		results[i] = AnomalyResult{
			IsAnomaly:   score > 3.5, // Approximately equivalent to 3-sigma
			Score:       score / 3.5,
			Probability: 2 * (1 - stat.NormalCDF(score, 0, 1)),
			ExpectedRange: Range{
				Lower: median - 3.5*mad,
				Upper: median + 3.5*mad,
			},
			Method:    "robust",
			Timestamp: points[i].Timestamp,
		}
	}

	return results
}

// ensembleResults combines results from multiple detection methods
func (d *AnomalyDetector) ensembleResults(results ...AnomalyResult) AnomalyResult {
	weights := map[string]float64{
		"statistical": 0.4,
		"seasonal":   0.3,
		"robust":     0.3,
	}

	var weightedScore float64
	var weightedProb float64
	var totalWeight float64

	for _, result := range results {
		if weight, ok := weights[result.Method]; ok {
			weightedScore += result.Score * weight
			weightedProb += result.Probability * weight
			totalWeight += weight
		}
	}

	if totalWeight > 0 {
		weightedScore /= totalWeight
		weightedProb /= totalWeight
	}

	// Combine ranges using weighted average
	var combinedRange Range
	for _, result := range results {
		if weight, ok := weights[result.Method]; ok {
			combinedRange.Lower += result.ExpectedRange.Lower * weight
			combinedRange.Upper += result.ExpectedRange.Upper * weight
		}
	}
	
	if totalWeight > 0 {
		combinedRange.Lower /= totalWeight
		combinedRange.Upper /= totalWeight
	}

	return AnomalyResult{
		IsAnomaly:     weightedScore > 1.0,
		Score:         weightedScore,
		Probability:   weightedProb,
		ExpectedRange: combinedRange,
		Method:        "ensemble",
		Timestamp:     results[0].Timestamp,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

import (
	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/stat"
)

// AnomalyDetector implements various anomaly detection algorithms
type AnomalyDetector struct {
	// Configuration
	MinDataPoints    int
	ConfidenceLevel float64
	SeasonalPeriod  int // For seasonal data (e.g., 24 for hourly data with daily patterns)
}

func NewAnomalyDetector(minDataPoints int, confidenceLevel float64, seasonalPeriod int) *AnomalyDetector {
	return &AnomalyDetector{
		MinDataPoints:    minDataPoints,
		ConfidenceLevel: confidenceLevel,
		SeasonalPeriod:  seasonalPeriod,
	}
}

// TimeSeriesPoint represents a data point in time series
type TimeSeriesPoint struct {
	Timestamp time.Time
	Value     float64
}

// AnomalyResult represents the result of anomaly detection
type AnomalyResult struct {
	IsAnomaly       bool
	Score           float64
	ExpectedRange   Range
	DeviationFactor float64
}

type Range struct {
	Lower float64
	Upper float64
}

// DetectAnomalies uses multiple methods to detect anomalies
func (d *AnomalyDetector) DetectAnomalies(points []TimeSeriesPoint) []AnomalyResult {
	if len(points) < d.MinDataPoints {
		return make([]AnomalyResult, len(points))
	}

	// Get results from different methods
	zscore := d.zScoreDetection(points)
	iqr := d.iqrDetection(points)
	seasonal := d.seasonalDecomposition(points)

	// Combine results using ensemble method
	results := make([]AnomalyResult, len(points))
	for i := range points {
		results[i] = d.ensembleResults(zscore[i], iqr[i], seasonal[i])
	}

	return results
}

// Z-Score based anomaly detection
func (d *AnomalyDetector) zScoreDetection(points []TimeSeriesPoint) []AnomalyResult {
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Value
	}

	mean, std := stat.MeanStdDev(values, nil)
	threshold := stat.InvNormalCDF(1-(1-d.ConfidenceLevel)/2) // Two-tailed test

	results := make([]AnomalyResult, len(points))
	for i, v := range values {
		zscore := math.Abs((v - mean) / std)
		results[i] = AnomalyResult{
			IsAnomaly: zscore > threshold,
			Score:     zscore / threshold,
			ExpectedRange: Range{
				Lower: mean - threshold*std,
				Upper: mean + threshold*std,
			},
			DeviationFactor: zscore,
		}
	}

	return results
}

// IQR based anomaly detection
func (d *AnomalyDetector) iqrDetection(points []TimeSeriesPoint) []AnomalyResult {
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Value
	}
	sort.Float64s(values)

	q1 := quantile(values, 0.25)
	q3 := quantile(values, 0.75)
	iqr := q3 - q1
	lowerBound := q1 - 1.5*iqr
	upperBound := q3 + 1.5*iqr

	results := make([]AnomalyResult, len(points))
	for i, p := range points {
		deviation := 0.0
		if p.Value < lowerBound {
			deviation = (lowerBound - p.Value) / iqr
		} else if p.Value > upperBound {
			deviation = (p.Value - upperBound) / iqr
		}

		results[i] = AnomalyResult{
			IsAnomaly: deviation > 0,
			Score:     deviation,
			ExpectedRange: Range{
				Lower: lowerBound,
				Upper: upperBound,
			},
			DeviationFactor: deviation,
		}
	}

	return results
}

// Seasonal decomposition and anomaly detection
func (d *AnomalyDetector) seasonalDecomposition(points []TimeSeriesPoint) []AnomalyResult {
	if len(points) < 2*d.SeasonalPeriod {
		return make([]AnomalyResult, len(points))
	}

	// Calculate seasonal components
	seasonal := make([]float64, d.SeasonalPeriod)
	for i := 0; i < d.SeasonalPeriod; i++ {
		var sum float64
		count := 0
		for j := i; j < len(points); j += d.SeasonalPeriod {
			sum += points[j].Value
			count++
		}
		seasonal[i] = sum / float64(count)
	}

	// Calculate trend using moving average
	trend := d.calculateTrend(points)

	// Calculate residuals and detect anomalies
	results := make([]AnomalyResult, len(points))
	residuals := make([]float64, len(points))

	for i, p := range points {
		seasonalIdx := i % d.SeasonalPeriod
		expected := trend[i] + seasonal[seasonalIdx]
		residuals[i] = p.Value - expected
	}

	// Calculate residual statistics
	mean, std := stat.MeanStdDev(residuals, nil)
	threshold := stat.InvNormalCDF(1-(1-d.ConfidenceLevel)/2) * std

	for i, r := range residuals {
		deviation := math.Abs(r - mean)
		results[i] = AnomalyResult{
			IsAnomaly: deviation > threshold,
			Score:     deviation / threshold,
			ExpectedRange: Range{
				Lower: trend[i] + seasonal[i%d.SeasonalPeriod] - threshold,
				Upper: trend[i] + seasonal[i%d.SeasonalPeriod] + threshold,
			},
			DeviationFactor: deviation / std,
		}
	}

	return results
}

// Helper functions
func (d *AnomalyDetector) calculateTrend(points []TimeSeriesPoint) []float64 {
	windowSize := d.SeasonalPeriod
	trend := make([]float64, len(points))

	for i := range points {
		start := max(0, i-windowSize/2)
		end := min(len(points), i+windowSize/2+1)
		
		sum := 0.0
		count := 0
		for j := start; j < end; j++ {
			sum += points[j].Value
			count++
		}
		trend[i] = sum / float64(count)
	}

	return trend
}

func (d *AnomalyDetector) ensembleResults(results ...AnomalyResult) AnomalyResult {
	if len(results) == 0 {
		return AnomalyResult{}
	}

	// Weight the scores from different methods
	weights := []float64{0.4, 0.3, 0.3} // Weights for z-score, IQR, and seasonal
	totalScore := 0.0
	totalWeight := 0.0

	for i, result := range results {
		if i < len(weights) {
			totalScore += result.Score * weights[i]
			totalWeight += weights[i]
		}
	}

	avgScore := totalScore / totalWeight
	
	// Combine ranges
	var combinedRange Range
	validRanges := 0
	for _, result := range results {
		if result.ExpectedRange.Lower != result.ExpectedRange.Upper {
			if validRanges == 0 {
				combinedRange = result.ExpectedRange
			} else {
				combinedRange.Lower = math.Max(combinedRange.Lower, result.ExpectedRange.Lower)
				combinedRange.Upper = math.Min(combinedRange.Upper, result.ExpectedRange.Upper)
			}
			validRanges++
		}
	}

	return AnomalyResult{
		IsAnomaly:       avgScore > 1.0,
		Score:           avgScore,
		ExpectedRange:   combinedRange,
		DeviationFactor: avgScore,
	}
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	
	pos := q * float64(len(sorted)-1)
	fpos := math.Floor(pos)
	ipos := int(fpos)
	
	if ipos+1 < len(sorted) {
		delta := pos - fpos
		return sorted[ipos]*(1-delta) + sorted[ipos+1]*delta
	}
	return sorted[ipos]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
