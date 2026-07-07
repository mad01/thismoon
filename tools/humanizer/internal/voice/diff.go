package voice

import "math"

// MetricDelta is a single metric's value in draft, value in sample, and
// the absolute difference. direction is "draft-higher", "sample-higher",
// or "same".
type MetricDelta struct {
	Metric    string  `json:"metric"`
	Draft     float64 `json:"draft"`
	Sample    float64 `json:"sample"`
	Delta     float64 `json:"delta"`
	Direction string  `json:"direction"`
}

// Diff compares draft and sample profiles across every scalar metric.
// Results are ordered by absolute delta descending so the biggest
// divergences surface first.
type Diff struct {
	Metrics []MetricDelta `json:"metrics"`
}

// DiffProfiles produces a Diff between two profiles.
func DiffProfiles(draft, sample Profile) Diff {
	metrics := []MetricDelta{
		delta("sentence_length_mean", draft.SentenceLengthMean, sample.SentenceLengthMean),
		delta("sentence_length_stddev", draft.SentenceLengthStdDev, sample.SentenceLengthStdDev),
		delta(
			"sentence_length_p50",
			float64(draft.SentenceLengthP50),
			float64(sample.SentenceLengthP50),
		),
		delta(
			"sentence_length_p90",
			float64(draft.SentenceLengthP90),
			float64(sample.SentenceLengthP90),
		),
		delta("em_dash_density_per_100_words", draft.EmDashDensity, sample.EmDashDensity),
		delta("semicolon_density_per_100_words", draft.SemicolonDensity, sample.SemicolonDensity),
		delta("colon_density_per_100_words", draft.ColonDensity, sample.ColonDensity),
		delta("paren_density_per_100_words", draft.ParenDensity, sample.ParenDensity),
		delta("comma_density_per_100_words", draft.CommaDensity, sample.CommaDensity),
		delta(
			"hyphenated_pair_density_per_100_words",
			draft.HyphenatedDensity,
			sample.HyphenatedDensity,
		),
		delta("bold_density_per_100_words", draft.BoldDensity, sample.BoldDensity),
		delta("contraction_rate_per_100_words", draft.ContractionRate, sample.ContractionRate),
		delta("type_token_ratio", draft.TypeTokenRatio, sample.TypeTokenRatio),
		delta("flesch_reading_ease", draft.FleschReadingEase, sample.FleschReadingEase),
	}
	// Sort by magnitude of delta.
	for i := 0; i < len(metrics); i++ {
		for j := i + 1; j < len(metrics); j++ {
			if math.Abs(metrics[j].Delta) > math.Abs(metrics[i].Delta) {
				metrics[i], metrics[j] = metrics[j], metrics[i]
			}
		}
	}
	return Diff{Metrics: metrics}
}

func delta(name string, draft, sample float64) MetricDelta {
	d := draft - sample
	dir := "same"
	if d > 0 {
		dir = "draft-higher"
	} else if d < 0 {
		dir = "sample-higher"
	}
	return MetricDelta{
		Metric:    name,
		Draft:     draft,
		Sample:    sample,
		Delta:     d,
		Direction: dir,
	}
}
